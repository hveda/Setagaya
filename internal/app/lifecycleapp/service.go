// Package lifecycleapp holds the test-lifecycle use-cases: deploy engines,
// trigger a run, stop it, purge the engines, and report status/detail. It
// orchestrates the domain (engine config building, run state machine) over the
// Scheduler, Executor, ObjectStore, and repository ports; it performs no I/O
// of its own.
package lifecycleapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/heridotlife/honryu/internal/domain/compile"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/shard"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// ErrNoTestFile is returned when a scenario to be triggered has no JMX test file.
var ErrNoTestFile = errors.New("lifecycle: scenario has no test file")

// Repo is the persistence the lifecycle use-cases need: read access to the
// execution, its execution scenarios and files, plus run/running-scenario state.
type Repo interface {
	GetProject(ctx context.Context, id int64) (project.Project, error)
	GetExecution(ctx context.Context, id int64) (execution.Execution, error)
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
	GetScenario(ctx context.Context, id int64) (scenario.Scenario, error)
	ScenarioFilesFor(ctx context.Context, scenarioID int64) (ports.ScenarioFiles, error)
	ExecutionFilesFor(ctx context.Context, executionID int64) ([]string, error)
	ports.RunRepository
}

// Metrics is the metric use-case's hook into the lifecycle. With measurements
// pushed, there is no collection to start or stop -- pods send while they run
// and stop when they are gone -- so what remains is dropping a purged
// execution's series, and finalising a run's report when Honryu itself ends it
// rather than letting it finish on its own.
//
// The metricsapp service implements it; a no-op default is used when none is
// wired (e.g. in tests that don't exercise metrics).
type Metrics interface {
	Purge(executionID int64)
	// Finalize writes the report for a run Honryu is deliberately ending.
	// Idempotent: a run already finalised by its own natural completion is left
	// untouched.
	Finalize(ctx context.Context, executionID, runID int64) error
}

type noopMetrics struct{}

func (noopMetrics) Purge(int64)                                  {}
func (noopMetrics) Finalize(context.Context, int64, int64) error { return nil }

// Usage records the usage of a run: a launch opened on trigger and closed on
// teardown. The usageapp service implements it; a no-op default is used when
// none is wired.
type Usage interface {
	RecordStart(ctx context.Context, executionID int64, owner string, engines, vu int) error
	RecordFinish(ctx context.Context, executionID int64, vu int) error
}

type noopUsage struct{}

func (noopUsage) RecordStart(context.Context, int64, string, int, int) error { return nil }
func (noopUsage) RecordFinish(context.Context, int64, int) error             { return nil }

// Service implements the lifecycle use-cases.
type Service struct {
	repo  Repo
	sched ports.Scheduler
	store ports.ObjectStore
	image ImageResolver
	// defaultEngine runs an execution that names none.
	defaultEngine taurus.Executor
	metrics       Metrics
	usage         Usage
}

// ImageResolver returns the container image for an engine. An empty engine
// means the execution expressed no preference and takes the deployment default.
type ImageResolver func(engine taurus.Executor) (string, error)

// WithDefaultEngine sets the engine used when an execution names none.
func (s *Service) WithDefaultEngine(e taurus.Executor) *Service {
	s.defaultEngine = e
	return s
}

// StaticImage is an ImageResolver that always answers with the same image,
// for tests and single-engine deployments.
func StaticImage(image string) ImageResolver {
	return func(taurus.Executor) (string, error) { return image, nil }
}

// NewService wires the lifecycle service. image resolves an execution's engine
// to the container image its pods run.
func NewService(repo Repo, sched ports.Scheduler, store ports.ObjectStore, image ImageResolver) *Service {
	return &Service{repo: repo, sched: sched, store: store, image: image, defaultEngine: taurus.ExecutorJMeter, metrics: noopMetrics{}, usage: noopUsage{}}
}

// WithMetrics attaches the metric collector started on trigger and stopped on
// teardown/purge. Returns the receiver for chaining.
func (s *Service) WithMetrics(m Metrics) *Service {
	if m != nil {
		s.metrics = m
	}
	return s
}

// WithUsage attaches the usage recorder. Returns the receiver for chaining.
func (s *Service) WithUsage(u Usage) *Service {
	if u != nil {
		s.usage = u
	}
	return s
}

// Deploy provisions the engines for every scenario of an execution. It is rejected
// while a run is in progress and is otherwise idempotent.
func (s *Service) Deploy(ctx context.Context, executionID int64) error {
	coll, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return err
	}
	if len(scenarios) == 0 {
		return run.ErrNoScenarios
	}
	if _, running, err := s.repo.CurrentRun(ctx, executionID); err != nil {
		return err
	} else if err := run.CanDeploy(run.DerivePhase(0, running)); err != nil {
		return err
	}
	// Resolve once, before deploying anything: an execution whose engine this
	// deployment has no image for must fail before half its scenarios are up.
	image, err := s.image(coll.Engine)
	if err != nil {
		return err
	}
	for _, ep := range scenarios {
		shards, err := shard.Plan(ep, ep.Engines)
		if err != nil {
			return err
		}
		specs, files, err := s.compileShards(ctx, coll, ep, shards, engineOf(coll, s.defaultEngine))
		if err != nil {
			return err
		}
		spec := ports.DeploySpec{
			// Empty is this deployment's own cluster, the only one there is until
			// Phase 8 records a cluster on the execution and maps refs to
			// credentials. The parameter exists now so that change adds a lookup
			// rather than a signature.
			Cluster:       "",
			ProjectID:     coll.ProjectID,
			ExecutionID:   executionID,
			ScenarioID:    ep.ScenarioID,
			Image:         image,
			Shards:        specs,
			ScenarioFiles: files,
		}
		if err := s.sched.DeployScenario(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}

// Trigger starts a run across all deployed, ready engines. Engines must be
// deployed and reachable; every scenario must have a test file.
func (s *Service) Trigger(ctx context.Context, executionID int64) error {
	coll, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return err
	}
	ec := loadprofile.Profile{ExecutionID: executionID, Tests: scenarios, CSVSplit: coll.CSVSplit}

	if err := s.ensureTestFiles(ctx, scenarios); err != nil {
		return err
	}

	_, running, err := s.repo.CurrentRun(ctx, executionID)
	if err != nil {
		return err
	}
	status, err := s.sched.ExecutionStatus(ctx, "", executionID, planRefs(scenarios))
	if err != nil {
		return err
	}
	phase := run.DerivePhase(status.PoolSize, running)
	if err := run.CanTrigger(phase, ec, status.PoolSize); err != nil {
		return err
	}

	// StartRun's id identified the run to the engine agents; with the agent
	// protocol gone, the run is identified in metrics by the ingest path (task 21).
	if _, err := s.repo.StartRun(ctx, executionID); err != nil {
		return err
	}

	// Under Taurus there is no separate trigger step: a pod runs bzt and starts
	// generating load the moment it starts, so this records that the run is under
	// way rather than instructing engines. Wiring pods to their compiled config
	// is task 23; until then a deployed execution generates no load.
	var triggerErrs []error
	for _, ep := range scenarios {
		if err := s.repo.MarkScenarioRunning(ctx, executionID, ep.ScenarioID); err != nil {
			triggerErrs = append(triggerErrs, err)
		}
	}
	if len(triggerErrs) == len(scenarios) {
		// Every scenario failed: roll the run back so the execution is triggerable
		// again after the operator fixes the problem.
		_ = s.repo.StopRun(ctx, executionID)
	}
	if len(triggerErrs) > 0 {
		return fmt.Errorf("lifecycle: trigger errors: %w", errors.Join(triggerErrs...))
	}
	// Begin streaming metrics from the now-running engines and open a usage
	// launch (best effort: accounting must not fail a successful trigger).
	if proj, perr := s.repo.GetProject(ctx, coll.ProjectID); perr == nil {
		_ = s.usage.RecordStart(ctx, executionID, proj.Owner, ec.TotalEngines(), run.VirtualUsers(ec))
	}
	return nil
}

// Stop halts the run: it stops every engine and clears run state. A run must be
// in progress.
func (s *Service) Stop(ctx context.Context, executionID int64) error {
	_, running, err := s.repo.CurrentRun(ctx, executionID)
	if err != nil {
		return err
	}
	if err := run.CanStop(run.DerivePhase(0, running)); err != nil {
		return err
	}
	return s.teardown(ctx, executionID)
}

// Purge stops any in-progress run and removes all engines of an execution.
func (s *Service) Purge(ctx context.Context, executionID int64) error {
	runID, running, err := s.repo.CurrentRun(ctx, executionID)
	if err != nil {
		return err
	}
	if running {
		if err := s.teardown(ctx, executionID); err != nil {
			return err
		}
		// After teardown, before the pods that hold them are deleted: this is
		// the last moment engine logs exist anywhere but here.
		s.captureLogs(ctx, executionID, runID)
	}
	if err := s.sched.PurgeExecution(ctx, "", executionID); err != nil {
		return err
	}
	s.metrics.Purge(executionID)
	return nil
}

// runLogKey is the object-store key for one shard's engine log.
//
// Retention is a lifecycle policy on the underlying bucket (the same GCS/Nexus
// adapters already used for scenario and execution artefacts), not something
// this code tracks -- there is no TTL concept on ObjectStore, and adding a
// sweep would duplicate a capability object storage already has.
func runLogKey(runID, scenarioID int64, shard int) string {
	return fmt.Sprintf("run/%d/scenario-%d-shard-%d.log", runID, scenarioID, shard)
}

// captureLogs saves each shard's engine output before Purge deletes its pod,
// so a run stays diagnosable after the cluster no longer has it.
//
// Best effort throughout: a customer must be able to purge a broken execution
// even if a log capture fails, and a missing log degrades diagnosability
// rather than the run itself. One shard's failure does not stop the others
// from being tried.
func (s *Service) captureLogs(ctx context.Context, executionID, runID int64) {
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return
	}
	for _, ep := range scenarios {
		for i := 0; i < ep.Engines; i++ {
			log, err := s.sched.PodLog(ctx, "", executionID, ep.ScenarioID, i)
			if err != nil {
				continue
			}
			_ = s.store.Upload(ctx, runLogKey(runID, ep.ScenarioID, i), strings.NewReader(log))
		}
	}
}

// podScenarioPath is where a scenario's artefacts are mounted in an engine pod.
const podScenarioPath = "/honryu/config/scenario/"

// scenarioKey is the object-store key of one of a scenario's files.
func scenarioKey(scenarioID int64, filename string) string {
	return fmt.Sprintf("scenario/%d/%s", scenarioID, filename)
}

// compileShards turns one scenario's share of the load into a config per pod.
//
// Each shard gets its own config carrying only its fraction of the users, which
// is what makes the pods together produce the profile that was asked for. The
// scenario's own artefacts travel with them, since a native scenario's config
// points at a script the pod must be able to open.
func (s *Service) compileShards(
	ctx context.Context,
	exe execution.Execution,
	entry loadprofile.Entry,
	shards []shard.Shard,
	engine taurus.Executor,
) ([]ports.ShardSpec, map[string][]byte, error) {
	sc, err := s.repo.GetScenario(ctx, entry.ScenarioID)
	if err != nil {
		return nil, nil, err
	}
	pf, err := s.repo.ScenarioFilesFor(ctx, entry.ScenarioID)
	if err != nil {
		return nil, nil, err
	}
	if sc.Kind == scenario.KindNative && pf.TestFile == "" {
		return nil, nil, ErrNoTestFile
	}

	files := map[string][]byte{}
	for _, name := range append([]string{pf.TestFile}, pf.Data...) {
		if name == "" {
			continue
		}
		content, dlErr := s.store.Download(ctx, scenarioKey(entry.ScenarioID, name))
		if dlErr != nil {
			// Naming the file matters: "object not found" alone leaves an
			// operator guessing which of a scenario's artefacts is missing.
			return nil, nil, fmt.Errorf("lifecycleapp: scenario %d file %q: %w",
				entry.ScenarioID, name, dlErr)
		}
		files[name] = content
	}

	si := compile.ScenarioInput{Scenario: sc}
	if sc.Kind == scenario.KindNative {
		si.ScriptPath = podScenarioPath + pf.TestFile
	}
	for _, name := range pf.Data {
		si.DataPaths = append(si.DataPaths, podScenarioPath+name)
	}

	specs := make([]ports.ShardSpec, len(shards))
	for i, sh := range shards {
		// Only this shard's slice of the load: the pods together add up to the
		// requested profile.
		shardEntry := entry
		shardEntry.Concurrency = sh.Concurrency
		shardEntry.Throughput = sh.Throughput

		cfg, cErr := compile.Taurus(compile.Input{
			Execution: exe,
			Profile:   loadprofile.Profile{Tests: []loadprofile.Entry{shardEntry}},
			Engine:    engine,
			Scenarios: map[int64]compile.ScenarioInput{entry.ScenarioID: si},
		})
		if cErr != nil {
			return nil, nil, cErr
		}
		raw, mErr := yaml.Marshal(cfg)
		if mErr != nil {
			return nil, nil, fmt.Errorf("lifecycleapp: encode shard config: %w", mErr)
		}
		specs[i] = ports.ShardSpec{Index: sh.Index, Concurrency: sh.Concurrency, Config: raw}
	}
	return specs, files, nil
}

// engineOf is the execution's engine, or the deployment default when it named
// none.
func engineOf(exe execution.Execution, fallback taurus.Executor) taurus.Executor {
	if exe.Engine != "" {
		return exe.Engine
	}
	return fallback
}

// teardown stops metric execution and engines, and clears run/running-scenario
// state (best effort on the engine stop calls, which may already be gone).
func (s *Service) teardown(ctx context.Context, executionID int64) error {
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return err
	}
	// Engines are stopped by removing their pods (Purge), not by calling them.
	for _, ep := range scenarios {
		if err := s.repo.ClearScenarioRunning(ctx, executionID, ep.ScenarioID); err != nil {
			return err
		}
	}
	// Close the usage launch (best effort).
	vu := run.VirtualUsers(loadprofile.Profile{Tests: scenarios})
	_ = s.usage.RecordFinish(ctx, executionID, vu)

	// Finalise the report before the run's identity is cleared: Finalize needs
	// the run id, and StopRun is what forgets it. Best effort, matching
	// RecordFinish above -- a customer must be able to stop or purge a broken
	// execution even if writing its final report fails.
	if runID, running, err := s.repo.CurrentRun(ctx, executionID); err == nil && running {
		_ = s.metrics.Finalize(ctx, executionID, runID)
	}
	return s.repo.StopRun(ctx, executionID)
}

// ScenarioStatus is the lifecycle view of one scenario's engines.
type ScenarioStatus struct {
	ScenarioID      int64     `json:"scenario_id"`
	EnginesWanted   int       `json:"engines"`
	EnginesDeployed int       `json:"engines_deployed"`
	Reachable       bool      `json:"engines_reachable"`
	InProgress      bool      `json:"in_progress"`
	StartedTime     time.Time `json:"started_time,omitempty"`
}

// Status is the lifecycle view of an execution.
type Status struct {
	Phase     run.Phase        `json:"phase"`
	PoolSize  int              `json:"pool_size"`
	Scenarios []ScenarioStatus `json:"status"`
}

// Status reports the deployment/run status of an execution.
func (s *Service) Status(ctx context.Context, executionID int64) (Status, error) {
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return Status{}, err
	}
	sched, err := s.sched.ExecutionStatus(ctx, "", executionID, planRefs(scenarios))
	if err != nil {
		return Status{}, err
	}
	_, running, err := s.repo.CurrentRun(ctx, executionID)
	if err != nil {
		return Status{}, err
	}
	runningByScenario, err := s.runningByScenario(ctx, executionID)
	if err != nil {
		return Status{}, err
	}

	out := Status{Phase: run.DerivePhase(sched.PoolSize, running), PoolSize: sched.PoolSize}
	for _, pr := range sched.Scenarios {
		ps := ScenarioStatus{
			ScenarioID:      pr.ScenarioID,
			EnginesWanted:   pr.EnginesWanted,
			EnginesDeployed: pr.EnginesDeployed,
			Reachable:       pr.Reachable,
		}
		if started, ok := runningByScenario[pr.ScenarioID]; ok {
			ps.InProgress = true
			ps.StartedTime = started
		}
		out.Scenarios = append(out.Scenarios, ps)
	}
	return out, nil
}

// EnginesDetail reports the engine pods and ingress of an execution.
func (s *Service) EnginesDetail(ctx context.Context, projectID, executionID int64) (ports.ExecutionDetail, error) {
	return s.sched.EngineDetail(ctx, "", projectID, executionID)
}

// PodLog returns the logs of a scenario's engine pod.
func (s *Service) PodLog(ctx context.Context, executionID, scenarioID int64) (string, error) {
	return s.sched.PodLog(ctx, "", executionID, scenarioID, 0)
}

// Resume returns the scenarios still marked running in this deployment context, so
// a restarted controller can re-establish tracking.
func (s *Service) Resume(ctx context.Context) ([]ports.RunningScenario, error) {
	return s.repo.RunningScenarios(ctx)
}

// --- helpers ----------------------------------------------------------------

func (s *Service) ensureTestFiles(ctx context.Context, scenarios []loadprofile.Entry) error {
	for _, ep := range scenarios {
		pf, err := s.repo.ScenarioFilesFor(ctx, ep.ScenarioID)
		if err != nil {
			return err
		}
		if pf.TestFile == "" {
			return fmt.Errorf("%w: scenario %d", ErrNoTestFile, ep.ScenarioID)
		}
	}
	return nil
}

func (s *Service) runningByScenario(ctx context.Context, executionID int64) (map[int64]time.Time, error) {
	rps, err := s.repo.RunningScenariosByExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]time.Time, len(rps))
	for _, rp := range rps {
		out[rp.ScenarioID] = rp.StartedTime
	}
	return out, nil
}

func planRefs(scenarios []loadprofile.Entry) []ports.ScenarioRef {
	refs := make([]ports.ScenarioRef, 0, len(scenarios))
	for _, ep := range scenarios {
		refs = append(refs, ports.ScenarioRef{ScenarioID: ep.ScenarioID, Shards: ep.Engines})
	}
	return refs
}
