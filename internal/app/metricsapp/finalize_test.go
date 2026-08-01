package metricsapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// finalBatch builds a shard's last batch, carrying its exit code.
func finalBatch(e *env, shard int, exitCode int) metrics.Batch {
	b := batch(e, shard, 1, 2)
	b.Final = true
	b.ExitCode = &exitCode
	return b
}

// The whole point of accumulating a run's measurements: once every shard has
// said it is done, the report exists on its own, without anyone calling Stop.
func TestIngest_NaturalCompletionFinalizesTheReport(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()

	if err := e.svc.Ingest(ctx, finalBatch(e, 0, 0)); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	rep, err := e.reports.GetReport(ctx, e.runID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if rep.ExecutionID != e.executionID || rep.RunID != e.runID {
		t.Fatalf("report identity = %+v", rep)
	}
	if rep.Outcome != taurus.OutcomePassed {
		t.Errorf("outcome = %q, want passed", rep.Outcome)
	}
	if rep.ScenarioID != e.scenarioIDs[0] {
		t.Errorf("scenario id = %d, want %d (single scenario)", rep.ScenarioID, e.scenarioIDs[0])
	}
	if rep.Achieved.Samples == 0 {
		t.Error("report has no achieved samples")
	}

	states, err := e.progress.ShardStates(ctx, e.runID)
	if err != nil {
		t.Fatalf("ShardStates: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("working state survived finalisation: %+v", states)
	}
}

// A sharded run's outcome is the most severe of its shards': one shard's
// criteria failure means the target failed, no matter that the other passed.
func TestIngest_NaturalCompletionCombinesShardExitCodes(t *testing.T) {
	t.Parallel()
	e := setup(t, 2) // one scenario, two shards
	ctx := context.Background()

	if err := e.svc.Ingest(ctx, finalBatch(e, 0, 0)); err != nil {
		t.Fatalf("Ingest shard 0: %v", err)
	}
	if err := e.svc.Ingest(ctx, finalBatch(e, 1, 3)); err != nil {
		t.Fatalf("Ingest shard 1: %v", err)
	}

	rep, err := e.reports.GetReport(ctx, e.runID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if rep.Outcome != taurus.OutcomeFailed {
		t.Errorf("outcome = %q, want failed", rep.Outcome)
	}
}

// A run does not finalise until every shard its load profile called for has
// finished -- one shard's Final must not be mistaken for the whole run's.
func TestIngest_DoesNotFinalizeUntilEveryShardIsDone(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	ctx := context.Background()

	if err := e.svc.Ingest(ctx, finalBatch(e, 0, 0)); err != nil {
		t.Fatalf("Ingest shard 0: %v", err)
	}
	if _, err := e.reports.GetReport(ctx, e.runID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("GetReport before every shard finished = %v, want ErrNotFound", err)
	}
}

// Natural completion and a later Honryu-initiated Finalize (teardown, arriving
// after the fact) race to finalise the same run. Whichever gets there first
// decides its outcome, and the report must survive the race unedited.
func TestFinalize_DoesNotOverwriteANaturallyCompletedReport(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()

	if err := e.svc.Ingest(ctx, finalBatch(e, 0, 0)); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if err := e.svc.Finalize(ctx, e.executionID, e.runID); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	rep, err := e.reports.GetReport(ctx, e.runID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if rep.Outcome != taurus.OutcomePassed {
		t.Errorf("outcome = %q, want the natural completion's passed -- Finalize must not overwrite it with aborted", rep.Outcome)
	}
}

// An execution can bundle several scenarios under one run. ScenarioID is only
// unambiguous when there is exactly one; the per-request label breakdown
// already covers the rest.
func TestFinalize_MultiScenarioLeavesScenarioIDUnset(t *testing.T) {
	t.Parallel()
	e := setup(t, 1, 1) // two scenarios, one shard each
	ctx := context.Background()

	if err := e.svc.Finalize(ctx, e.executionID, e.runID); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	rep, err := e.reports.GetReport(ctx, e.runID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if rep.ScenarioID != 0 {
		t.Errorf("scenario id = %d, want 0 (ambiguous across %d scenarios)", rep.ScenarioID, len(e.scenarioIDs))
	}
	if rep.Outcome != taurus.OutcomeAborted {
		t.Errorf("outcome = %q, want aborted", rep.Outcome)
	}
}

// Concurrency sums exactly as usage accounting already collapses a profile
// (run.VirtualUsers); throughput sums since each scenario's rate is additive;
// duration takes the longest, since the run lasts as long as its longest
// scenario.
func TestFinalize_RequestedLoadCollapsesMultipleScenarios(t *testing.T) {
	t.Parallel()
	e := setup(t, 2, 3) // two scenarios: 2 engines and 3 engines
	ctx := context.Background()

	profile := []loadprofile.Entry{
		{ScenarioID: e.scenarioIDs[0], Concurrency: 10, Engines: 2, Rampup: 1, Duration: 30, Throughput: 100},
		{ScenarioID: e.scenarioIDs[1], Concurrency: 5, Engines: 3, Rampup: 1, Duration: 90, Throughput: 50},
	}
	if err := e.store.StoreLoadProfile(ctx, e.executionID, false, profile); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}

	if err := e.svc.Finalize(ctx, e.executionID, e.runID); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	rep, err := e.reports.GetReport(ctx, e.runID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	// VirtualUsers: 10*2 + 5*3 = 35.
	if rep.Requested.Concurrency != 35 {
		t.Errorf("requested concurrency = %d, want 35", rep.Requested.Concurrency)
	}
	if rep.Requested.Throughput != 150 {
		t.Errorf("requested throughput = %v, want 150", rep.Requested.Throughput)
	}
	if rep.Requested.DurationSeconds != 90 {
		t.Errorf("requested duration = %d, want 90 (the longer scenario)", rep.Requested.DurationSeconds)
	}
}

func TestFinalize_PropagatesAReportStoreSaveFailure(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	e.reports.SaveErr = errors.New("boom")

	if err := e.svc.Finalize(context.Background(), e.executionID, e.runID); err == nil {
		t.Error("Finalize succeeded despite SaveReport failing")
	}
}

// A store error other than "not found" is not the same as "never finalised":
// it must stop finalisation rather than be mistaken for a fresh run.
func TestFinalize_PropagatesAReportStoreReadFailure(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	e.reports.GetErr = errors.New("boom")

	if err := e.svc.Finalize(context.Background(), e.executionID, e.runID); err == nil {
		t.Error("Finalize succeeded despite GetReport failing")
	}
}

func TestFinalize_UnknownRunPropagatesRunHistoryError(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)

	if err := e.svc.Finalize(context.Background(), e.executionID, e.runID+999); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Finalize(unknown run) = %v, want ErrNotFound from RunHistory", err)
	}
}

// The clock a report is stamped with is injectable, so a test can assert on it
// rather than a moving target.
func TestFinalize_UsesTheInjectedClock(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	fixed := time.Unix(5000, 0).UTC()
	e.svc.WithNow(func() time.Time { return fixed })

	if err := e.svc.Finalize(context.Background(), e.executionID, e.runID); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	rep, err := e.reports.GetReport(context.Background(), e.runID)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if !rep.EndedAt.Equal(fixed) {
		t.Errorf("EndedAt = %v, want %v", rep.EndedAt, fixed)
	}
}
