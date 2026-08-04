// Package metricsapp is the metric use-case: it absorbs the measurements engine
// pods push, stamps each with the pod that produced it, fans it to the
// MetricsSink (Prometheus) and the EventBus (SSE subscribers) for the live view,
// and accumulates it into the run's report.
//
// Measurements used to be pulled: the controller opened a stream to every
// engine's agent, which meant tracking which executions were being collected,
// re-establishing those streams after a restart, and losing whatever a pod
// measured once it became unreachable. Under Taurus a sidecar in each pod pushes
// instead, so none of that machinery has anything to do -- an unreachable pod is
// simply one that has stopped sending, and what it sent already arrived.
//
// What remains is dropping an execution's series when it is purged, and
// finalising a run's report once it is over -- naturally, when every shard has
// said it is done, or because Honryu itself is ending it.
package metricsapp

import (
	"context"
	"time"

	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// Repo is the persistence the service reads to attribute a pushed batch and to
// finalise a run's report.
type Repo interface {
	// GetExecution supplies a report's Engine, from the execution's own
	// configured preference.
	GetExecution(ctx context.Context, executionID int64) (execution.Execution, error)
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
	CurrentRun(ctx context.Context, executionID int64) (int64, bool, error)
	// RunHistory supplies a report's StartedAt: nothing else keeps when a run
	// began once it is no longer the active one.
	RunHistory(ctx context.Context, runID int64) (ports.RunRecord, error)
}

// Service absorbs pushed measurements and finalises runs.
type Service struct {
	repo     Repo
	sink     ports.MetricsSink
	bus      ports.EventBus
	progress ports.ReportProgress
	reports  ports.ReportStore
	// seen deduplicates intervals a pod pushed more than once, for the live
	// view only. The permanent record's exactness comes from ReportProgress's
	// own per-shard sequence, which survives a restart this map does not.
	seen *seen
	now  func() time.Time
}

// NewService wires the metric service.
func NewService(repo Repo, sink ports.MetricsSink, bus ports.EventBus, progress ports.ReportProgress, reports ports.ReportStore) *Service {
	return &Service{repo: repo, sink: sink, bus: bus, progress: progress, reports: reports, seen: newSeen(), now: time.Now}
}

// WithNow overrides the clock a finalised report is stamped with. Returns the
// receiver for chaining.
func (s *Service) WithNow(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// Purge drops an execution's metric series and forgets what it absorbed.
//
// Called when an execution's engines are removed. Without it a long-lived
// controller would hold a series for every execution it had ever run.
func (s *Service) Purge(executionID int64) {
	s.sink.DeleteExecution(executionID)
	s.seen.forget(executionID)
}

// Finalize writes the report for a run Honryu is deliberately ending -- a
// user-initiated Stop or Purge -- rather than one that finished on its own.
//
// Idempotent: a run already finalised by its own natural completion is left
// untouched. That is what stops a Purge called after a run has already
// finished from overwriting its real verdict with "aborted" -- teardown and
// natural completion are racing to finalise the same run, and whichever gets
// there first decides it. The guarantee comes from SaveReport itself (the
// first report saved for a run is the one that survives), not from a check
// here first: a plain existence check followed later by a save would leave a
// window where both racers could pass the check before either had written.
func (s *Service) Finalize(ctx context.Context, executionID, runID int64) error {
	outcome, err := s.stopOutcome(ctx, runID)
	if err != nil {
		return err
	}
	return s.finalize(ctx, executionID, runID, outcome)
}

// stopOutcome is the outcome for a run Honryu is deliberately ending.
//
// Not derived from shard exit codes the way finalizeCompleted's is:
// taurus.OutcomeFromExitCode's own doc comment establishes that bzt's real
// exit codes cannot tell a deliberate stop from a crash, so Honryu's own
// certainty that it issued the stop -- OutcomeAborted -- is the baseline, not
// something inferred here. But a shard that had already finished naturally,
// with a real exit code, in the race between Stop and that shard's own last
// Final batch is real evidence and must not be silently discarded just
// because Stop reached the run first: if that evidence is more severe than an
// ordinary abort -- a criteria failure or an engine error -- it must win.
func (s *Service) stopOutcome(ctx context.Context, runID int64) (taurus.Outcome, error) {
	states, err := s.progress.ShardStates(ctx, runID)
	if err != nil {
		return "", err
	}
	outcomes := []taurus.Outcome{taurus.OutcomeAborted}
	for _, st := range states {
		if st.Finished && st.ExitCode != nil {
			outcomes = append(outcomes, taurus.OutcomeFromExitCode(*st.ExitCode))
		}
	}
	return taurus.WorstOutcome(outcomes), nil
}

// finalize builds a run's report from its accumulated measurements, stores it,
// and discards the working state that produced it.
//
// Discard runs whether this call's SaveReport actually wrote the report or
// found one already there: either way the working state this run produced is
// no longer needed, and running it unconditionally means a retry after a
// prior Discard failure still cleans up rather than short-circuiting on an
// early "already finalised" check the way a return-before-Discard would.
func (s *Service) finalize(ctx context.Context, executionID, runID int64, outcome taurus.Outcome) error {
	snapshot, err := s.progress.Snapshot(ctx, runID)
	if err != nil {
		return err
	}
	profile, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return err
	}
	history, err := s.repo.RunHistory(ctx, runID)
	if err != nil {
		return err
	}
	exe, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	meta := report.Meta{
		ExecutionID: executionID,
		RunID:       runID,
		// The execution's own configured preference. Empty when it deferred to
		// the deployment's default engine instead of naming one -- that
		// resolution happens in lifecycleapp at deploy time and is not
		// currently threaded through to here, so a defaulted execution's
		// report still under-reports which engine actually ran.
		Engine:    exe.Engine,
		StartedAt: history.StartedTime,
		EndedAt:   s.now(),
		Requested: requestedLoad(profile),
		Outcome:   outcome,
	}
	// An execution can bundle several scenarios under one run; ScenarioID is
	// informational and only unambiguous when there is exactly one. The label
	// breakdown the report already carries covers the multi-scenario case.
	if len(profile) == 1 {
		meta.ScenarioID = profile[0].ScenarioID
	}

	rep := report.Restore(snapshot).Report(meta)
	if err := s.reports.SaveReport(ctx, rep); err != nil {
		return err
	}
	return s.progress.Discard(ctx, runID)
}

// requestedLoad collapses an execution's load profile into the one figure a
// report compares achieved load against. An execution can bundle several
// scenarios, each with its own rate: concurrency sums exactly as usage
// accounting already collapses it (run.VirtualUsers), throughput sums since
// each scenario's target rate is additive, and duration takes the longest,
// since the run lasts as long as its longest scenario.
func requestedLoad(profile []loadprofile.Entry) report.Load {
	load := report.Load{Concurrency: run.VirtualUsers(loadprofile.Profile{Tests: profile})}
	for _, e := range profile {
		load.Throughput += float64(e.Throughput)
		if e.Duration > load.DurationSeconds {
			load.DurationSeconds = e.Duration
		}
	}
	return load
}
