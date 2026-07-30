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

	"github.com/heridotlife/Setagaya/internal/domain/collection"
	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/plan"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/run"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// ErrNoTestFile is returned when a plan to be triggered has no JMX test file.
var ErrNoTestFile = errors.New("lifecycle: plan has no test file")

// Repo is the persistence the lifecycle use-cases need: read access to the
// collection, its execution plans and files, plus run/running-plan state.
type Repo interface {
	GetProject(ctx context.Context, id int64) (project.Project, error)
	GetCollection(ctx context.Context, id int64) (collection.Collection, error)
	ExecutionPlansFor(ctx context.Context, collectionID int64) ([]loadprofile.Entry, error)
	GetPlan(ctx context.Context, id int64) (plan.Plan, error)
	PlanFilesFor(ctx context.Context, planID int64) (ports.PlanFiles, error)
	CollectionFilesFor(ctx context.Context, collectionID int64) ([]string, error)
	ports.RunRepository
}

// Metrics is the metric-collection lifecycle the run drives: Start streaming on
// trigger, Stop on teardown, and Purge (stop + drop series) on purge. The
// metricsapp service implements it; a no-op default is used when none is wired
// (e.g. in tests that don't exercise metrics).
type Metrics interface {
	Start(collectionID int64)
	Stop(collectionID int64)
	Purge(collectionID int64)
}

type noopMetrics struct{}

func (noopMetrics) Start(int64) {}
func (noopMetrics) Stop(int64)  {}
func (noopMetrics) Purge(int64) {}

// Usage records the usage of a run: a launch opened on trigger and closed on
// teardown. The usageapp service implements it; a no-op default is used when
// none is wired.
type Usage interface {
	RecordStart(ctx context.Context, collectionID int64, owner string, engines, vu int) error
	RecordFinish(ctx context.Context, collectionID int64, vu int) error
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

// Deploy provisions the engines for every plan of a collection. It is rejected
// while a run is in progress and is otherwise idempotent.
func (s *Service) Deploy(ctx context.Context, collectionID int64) error {
	coll, err := s.repo.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	plans, err := s.repo.ExecutionPlansFor(ctx, collectionID)
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		return run.ErrNoPlans
	}
	if _, running, err := s.repo.CurrentRun(ctx, collectionID); err != nil {
		return err
	} else if err := run.CanDeploy(run.DerivePhase(0, running)); err != nil {
		return err
	}
	for _, ep := range plans {
		spec := ports.DeploySpec{
			ProjectID:    coll.ProjectID,
			CollectionID: collectionID,
			PlanID:       ep.PlanID,
			Engines:      ep.Engines,
			Image:        s.engineImage,
		}
		if err := s.sched.DeployPlan(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}

// Trigger starts a run across all deployed, ready engines. Engines must be
// deployed and reachable; every plan must have a test file.
func (s *Service) Trigger(ctx context.Context, collectionID int64) error {
	coll, err := s.repo.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	plans, err := s.repo.ExecutionPlansFor(ctx, collectionID)
	if err != nil {
		return err
	}
	ec := loadprofile.Profile{CollectionID: collectionID, Tests: plans, CSVSplit: coll.CSVSplit}

	if err := s.ensureTestFiles(ctx, plans); err != nil {
		return err
	}

	_, running, err := s.repo.CurrentRun(ctx, collectionID)
	if err != nil {
		return err
	}
	status, err := s.sched.CollectionStatus(ctx, collectionID, planRefs(plans))
	if err != nil {
		return err
	}
	phase := run.DerivePhase(status.PoolSize, running)
	if err := run.CanTrigger(phase, ec, status.PoolSize); err != nil {
		return err
	}

	runID, err := s.repo.StartRun(ctx, collectionID)
	if err != nil {
		return err
	}

	collData, err := s.collectionFiles(ctx, collectionID)
	if err != nil {
		return err
	}

	var triggerErrs []error
	for i, ep := range plans {
		if err := s.triggerPlan(ctx, coll, ec, collData, i, ep, runID); err != nil {
			triggerErrs = append(triggerErrs, err)
			continue
		}
		if err := s.repo.MarkPlanRunning(ctx, collectionID, ep.PlanID); err != nil {
			triggerErrs = append(triggerErrs, err)
		}
	}

	if len(triggerErrs) == len(plans) {
		// Every plan failed: roll the run back so the collection is triggerable
		// again after the operator fixes the problem.
		_ = s.repo.StopRun(ctx, collectionID)
	}
	if len(triggerErrs) > 0 {
		return fmt.Errorf("lifecycle: trigger errors: %w", errors.Join(triggerErrs...))
	}
	// Begin streaming metrics from the now-running engines and open a usage
	// launch (best effort: accounting must not fail a successful trigger).
	s.metrics.Start(collectionID)
	if proj, perr := s.repo.GetProject(ctx, coll.ProjectID); perr == nil {
		_ = s.usage.RecordStart(ctx, collectionID, proj.Owner, ec.TotalEngines(), run.VirtualUsers(ec))
	}
	return nil
}

func (s *Service) triggerPlan(ctx context.Context, coll collection.Collection, ec loadprofile.Profile, collData []engine.File, index int, ep loadprofile.Entry, runID int64) error {
	pf, err := s.repo.PlanFilesFor(ctx, ep.PlanID)
	if err != nil {
		return err
	}
	urls, err := s.sched.EngineURLs(ctx, coll.ID, ep.PlanID, ep.Engines)
	if err != nil {
		return err
	}
	in := engine.PlanInput{
		PlanIndex:          index,
		PlanCount:          len(ec.Tests),
		CollectionCSVSplit: coll.CSVSplit,
		CollectionData:     collData,
		Engines:            ep.Engines,
		Concurrency:        ep.Concurrency,
		Rampup:             ep.Rampup,
		Duration:           ep.Duration,
		CSVSplit:           ep.CSVSplit,
		TestFile:           s.planFile(ep.PlanID, pf.TestFile),
		PlanData:           s.planFiles(ep.PlanID, pf.Data),
		RunID:              runID,
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
func (s *Service) Stop(ctx context.Context, collectionID int64) error {
	_, running, err := s.repo.CurrentRun(ctx, collectionID)
	if err != nil {
		return err
	}
	if err := run.CanStop(run.DerivePhase(0, running)); err != nil {
		return err
	}
	return s.teardown(ctx, collectionID)
}

// Purge stops any in-progress run and removes all engines of a collection.
func (s *Service) Purge(ctx context.Context, collectionID int64) error {
	if _, running, err := s.repo.CurrentRun(ctx, collectionID); err != nil {
		return err
	} else if running {
		if err := s.teardown(ctx, collectionID); err != nil {
			return err
		}
	}
	if err := s.sched.PurgeCollection(ctx, collectionID); err != nil {
		return err
	}
	s.metrics.Purge(collectionID)
	return nil
}

// teardown stops metric collection and engines, and clears run/running-plan
// state (best effort on the engine stop calls, which may already be gone).
func (s *Service) teardown(ctx context.Context, collectionID int64) error {
	s.metrics.Stop(collectionID)
	plans, err := s.repo.ExecutionPlansFor(ctx, collectionID)
	if err != nil {
		return err
	}
	for _, ep := range plans {
		if urls, err := s.sched.EngineURLs(ctx, collectionID, ep.PlanID, ep.Engines); err == nil {
			for _, url := range urls {
				_ = s.exec.Stop(ctx, url)
			}
		}
		if err := s.repo.ClearPlanRunning(ctx, collectionID, ep.PlanID); err != nil {
			return err
		}
	}
	// Close the usage launch (best effort).
	vu := run.VirtualUsers(loadprofile.Profile{Tests: plans})
	_ = s.usage.RecordFinish(ctx, collectionID, vu)
	return s.repo.StopRun(ctx, collectionID)
}

// PlanStatus is the lifecycle view of one plan's engines.
type PlanStatus struct {
	PlanID          int64     `json:"plan_id"`
	EnginesWanted   int       `json:"engines"`
	EnginesDeployed int       `json:"engines_deployed"`
	Reachable       bool      `json:"engines_reachable"`
	InProgress      bool      `json:"in_progress"`
	StartedTime     time.Time `json:"started_time,omitempty"`
}

// Status is the lifecycle view of a collection.
type Status struct {
	Phase    run.Phase    `json:"phase"`
	PoolSize int          `json:"pool_size"`
	Plans    []PlanStatus `json:"status"`
}

// Status reports the deployment/run status of a collection.
func (s *Service) Status(ctx context.Context, collectionID int64) (Status, error) {
	plans, err := s.repo.ExecutionPlansFor(ctx, collectionID)
	if err != nil {
		return Status{}, err
	}
	sched, err := s.sched.CollectionStatus(ctx, collectionID, planRefs(plans))
	if err != nil {
		return Status{}, err
	}
	_, running, err := s.repo.CurrentRun(ctx, collectionID)
	if err != nil {
		return Status{}, err
	}
	runningByPlan, err := s.runningByPlan(ctx, collectionID)
	if err != nil {
		return Status{}, err
	}

	out := Status{Phase: run.DerivePhase(sched.PoolSize, running), PoolSize: sched.PoolSize}
	for _, pr := range sched.Plans {
		ps := PlanStatus{
			PlanID:          pr.PlanID,
			EnginesWanted:   pr.EnginesWanted,
			EnginesDeployed: pr.EnginesDeployed,
			Reachable:       pr.Reachable,
		}
		if started, ok := runningByPlan[pr.PlanID]; ok {
			ps.InProgress = true
			ps.StartedTime = started
		}
		out.Plans = append(out.Plans, ps)
	}
	return out, nil
}

// EnginesDetail reports the engine pods and ingress of a collection.
func (s *Service) EnginesDetail(ctx context.Context, projectID, collectionID int64) (ports.CollectionDetail, error) {
	return s.sched.EngineDetail(ctx, projectID, collectionID)
}

// PodLog returns the logs of a plan's engine pod.
func (s *Service) PodLog(ctx context.Context, collectionID, planID int64) (string, error) {
	return s.sched.PodLog(ctx, collectionID, planID)
}

// Resume returns the plans still marked running in this deployment context, so
// a restarted controller can re-establish tracking.
func (s *Service) Resume(ctx context.Context) ([]ports.RunningPlan, error) {
	return s.repo.RunningPlans(ctx)
}

// --- helpers ----------------------------------------------------------------

func (s *Service) ensureTestFiles(ctx context.Context, plans []loadprofile.Entry) error {
	for _, ep := range plans {
		pf, err := s.repo.PlanFilesFor(ctx, ep.PlanID)
		if err != nil {
			return err
		}
		if pf.TestFile == "" {
			return fmt.Errorf("%w: plan %d", ErrNoTestFile, ep.PlanID)
		}
	}
	return nil
}

func (s *Service) collectionFiles(ctx context.Context, collectionID int64) ([]engine.File, error) {
	names, err := s.repo.CollectionFilesFor(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	files := make([]engine.File, 0, len(names))
	for _, name := range names {
		key := fmt.Sprintf("collection/%d/%s", collectionID, name)
		files = append(files, engine.File{Filename: name, Filepath: key, Filelink: s.store.URL(key)})
	}
	return files, nil
}

func (s *Service) planFile(planID int64, name string) engine.File {
	key := fmt.Sprintf("plan/%d/%s", planID, name)
	return engine.File{Filename: name, Filepath: key, Filelink: s.store.URL(key)}
}

func (s *Service) planFiles(planID int64, names []string) []engine.File {
	files := make([]engine.File, 0, len(names))
	for _, name := range names {
		files = append(files, s.planFile(planID, name))
	}
	return files
}

func (s *Service) runningByPlan(ctx context.Context, collectionID int64) (map[int64]time.Time, error) {
	rps, err := s.repo.RunningPlansByCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]time.Time, len(rps))
	for _, rp := range rps {
		out[rp.PlanID] = rp.StartedTime
	}
	return out, nil
}

func planRefs(plans []loadprofile.Entry) []ports.PlanRef {
	refs := make([]ports.PlanRef, 0, len(plans))
	for _, ep := range plans {
		refs = append(refs, ports.PlanRef{PlanID: ep.PlanID, Engines: ep.Engines})
	}
	return refs
}
