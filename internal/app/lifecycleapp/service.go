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
	"time"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/run"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports"
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

// Metrics is the metric-execution lifecycle the run drives: Start streaming on
// trigger, Stop on teardown, and Purge (stop + drop series) on purge. The
// metricsapp service implements it; a no-op default is used when none is wired
// (e.g. in tests that don't exercise metrics).
type Metrics interface {
	Start(executionID int64)
	Stop(executionID int64)
	Purge(executionID int64)
}

type noopMetrics struct{}

func (noopMetrics) Start(int64) {}
func (noopMetrics) Stop(int64)  {}
func (noopMetrics) Purge(int64) {}

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
	repo        Repo
	sched       ports.Scheduler
	exec        ports.Executor
	store       ports.ObjectStore
	engineImage string
	metrics     Metrics
	usage       Usage
}

// NewService wires the lifecycle service. engineImage is the executor container
// image deployed for every engine.
func NewService(repo Repo, sched ports.Scheduler, exec ports.Executor, store ports.ObjectStore, engineImage string) *Service {
	return &Service{repo: repo, sched: sched, exec: exec, store: store, engineImage: engineImage, metrics: noopMetrics{}, usage: noopUsage{}}
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
	for _, ep := range scenarios {
		spec := ports.DeploySpec{
			ProjectID:   coll.ProjectID,
			ExecutionID: executionID,
			ScenarioID:  ep.ScenarioID,
			Engines:     ep.Engines,
			Image:       s.engineImage,
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
	status, err := s.sched.ExecutionStatus(ctx, executionID, planRefs(scenarios))
	if err != nil {
		return err
	}
	phase := run.DerivePhase(status.PoolSize, running)
	if err := run.CanTrigger(phase, ec, status.PoolSize); err != nil {
		return err
	}

	runID, err := s.repo.StartRun(ctx, executionID)
	if err != nil {
		return err
	}

	collData, err := s.collectionFiles(ctx, executionID)
	if err != nil {
		return err
	}

	var triggerErrs []error
	for i, ep := range scenarios {
		if err := s.triggerScenario(ctx, coll, ec, collData, i, ep, runID); err != nil {
			triggerErrs = append(triggerErrs, err)
			continue
		}
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
	s.metrics.Start(executionID)
	if proj, perr := s.repo.GetProject(ctx, coll.ProjectID); perr == nil {
		_ = s.usage.RecordStart(ctx, executionID, proj.Owner, ec.TotalEngines(), run.VirtualUsers(ec))
	}
	return nil
}

func (s *Service) triggerScenario(ctx context.Context, coll execution.Execution, ec loadprofile.Profile, collData []engine.File, index int, ep loadprofile.Entry, runID int64) error {
	pf, err := s.repo.ScenarioFilesFor(ctx, ep.ScenarioID)
	if err != nil {
		return err
	}
	urls, err := s.sched.EngineURLs(ctx, coll.ID, ep.ScenarioID, ep.Engines)
	if err != nil {
		return err
	}
	in := engine.PlanInput{
		ScenarioIndex:     index,
		ScenarioCount:     len(ec.Tests),
		ExecutionCSVSplit: coll.CSVSplit,
		ExecutionData:     collData,
		Engines:           ep.Engines,
		Concurrency:       ep.Concurrency,
		Rampup:            ep.Rampup,
		Duration:          ep.Duration,
		CSVSplit:          ep.CSVSplit,
		TestFile:          s.planFile(ep.ScenarioID, pf.TestFile),
		ScenarioData:      s.planFiles(ep.ScenarioID, pf.Data),
		RunID:             runID,
	}
	configs := engine.BuildConfigs(in)
	for i, cfg := range configs {
		if err := s.exec.Trigger(ctx, urls[i], cfg); err != nil {
			return err
		}
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
	if _, running, err := s.repo.CurrentRun(ctx, executionID); err != nil {
		return err
	} else if running {
		if err := s.teardown(ctx, executionID); err != nil {
			return err
		}
	}
	if err := s.sched.PurgeExecution(ctx, executionID); err != nil {
		return err
	}
	s.metrics.Purge(executionID)
	return nil
}

// teardown stops metric execution and engines, and clears run/running-scenario
// state (best effort on the engine stop calls, which may already be gone).
func (s *Service) teardown(ctx context.Context, executionID int64) error {
	s.metrics.Stop(executionID)
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return err
	}
	for _, ep := range scenarios {
		if urls, err := s.sched.EngineURLs(ctx, executionID, ep.ScenarioID, ep.Engines); err == nil {
			for _, url := range urls {
				_ = s.exec.Stop(ctx, url)
			}
		}
		if err := s.repo.ClearScenarioRunning(ctx, executionID, ep.ScenarioID); err != nil {
			return err
		}
	}
	// Close the usage launch (best effort).
	vu := run.VirtualUsers(loadprofile.Profile{Tests: scenarios})
	_ = s.usage.RecordFinish(ctx, executionID, vu)
	return s.repo.StopRun(ctx, executionID)
}

// ScenarioStatus is the lifecycle view of one scenario's engines.
type ScenarioStatus struct {
	ScenarioID      int64     `json:"plan_id"`
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
	sched, err := s.sched.ExecutionStatus(ctx, executionID, planRefs(scenarios))
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
	return s.sched.EngineDetail(ctx, projectID, executionID)
}

// PodLog returns the logs of a scenario's engine pod.
func (s *Service) PodLog(ctx context.Context, executionID, scenarioID int64) (string, error) {
	return s.sched.PodLog(ctx, executionID, scenarioID)
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

func (s *Service) collectionFiles(ctx context.Context, executionID int64) ([]engine.File, error) {
	names, err := s.repo.ExecutionFilesFor(ctx, executionID)
	if err != nil {
		return nil, err
	}
	files := make([]engine.File, 0, len(names))
	for _, name := range names {
		key := fmt.Sprintf("collection/%d/%s", executionID, name)
		files = append(files, engine.File{Filename: name, Filepath: key, Filelink: s.store.URL(key)})
	}
	return files, nil
}

func (s *Service) planFile(scenarioID int64, name string) engine.File {
	key := fmt.Sprintf("plan/%d/%s", scenarioID, name)
	return engine.File{Filename: name, Filepath: key, Filelink: s.store.URL(key)}
}

func (s *Service) planFiles(scenarioID int64, names []string) []engine.File {
	files := make([]engine.File, 0, len(names))
	for _, name := range names {
		files = append(files, s.planFile(scenarioID, name))
	}
	return files
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
		refs = append(refs, ports.ScenarioRef{ScenarioID: ep.ScenarioID, Engines: ep.Engines})
	}
	return refs
}
