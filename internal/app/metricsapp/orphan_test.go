package metricsapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/app/metricsapp"
)

// A shard Final that arrives with no open run is the stranded-run signal
// (engines finished while nobody triggered, task 121's live finding): it must
// be recorded as an orphan completion for Trigger's guard -- and the push
// itself still rejected exactly as before, so nothing downstream treats it as
// data.
func TestIngest_FinalWithNoRunRecordsOrphanCompletion(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()
	// Close the run setup() opened: the engines' Final now arrives orphaned.
	if err := e.store.StopRun(ctx, e.executionID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}

	err := e.svc.Ingest(ctx, finalBatch(e, 0, 3))
	if !errors.Is(err, metricsapp.ErrNoActiveRun) {
		t.Fatalf("Ingest orphaned final = %v, want ErrNoActiveRun", err)
	}

	orphans, err := e.store.OrphanCompletions(ctx, e.executionID)
	if err != nil {
		t.Fatalf("OrphanCompletions: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}
	oc := orphans[0]
	if oc.ExecutionID != e.executionID || oc.ScenarioID != e.scenarioIDs[0] || oc.ShardIndex != 0 {
		t.Fatalf("orphan = %+v, want this execution's shard 0 of scenario %d", oc, e.scenarioIDs[0])
	}
	if oc.ExitCode == nil || *oc.ExitCode != 3 {
		t.Fatalf("orphan exit code = %v, want 3", oc.ExitCode)
	}

	// A retried Final stays one event (the repository contract pins the
	// overwrite; this pins that Ingest keeps feeding it the same shard).
	if err := e.svc.Ingest(ctx, finalBatch(e, 0, 3)); !errors.Is(err, metricsapp.ErrNoActiveRun) {
		t.Fatalf("retried final = %v, want ErrNoActiveRun", err)
	}
	again, _ := e.store.OrphanCompletions(ctx, e.executionID)
	if len(again) != 1 {
		t.Fatalf("orphans after retry = %d, want 1", len(again))
	}
}

// Non-Final batches with no open run are just noise from a pod that outlived
// its run: rejected as before, and no orphan evidence recorded -- only a
// shard's last word says the engines are finished.
func TestIngest_NonFinalWithNoRunRecordsNothing(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()
	if err := e.store.StopRun(ctx, e.executionID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}

	if err := e.svc.Ingest(ctx, batch(e, 0, 1, 2)); !errors.Is(err, metricsapp.ErrNoActiveRun) {
		t.Fatalf("Ingest non-final = %v, want ErrNoActiveRun", err)
	}
	orphans, err := e.store.OrphanCompletions(ctx, e.executionID)
	if err != nil {
		t.Fatalf("OrphanCompletions: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("orphans = %d, want 0 (no Final, no evidence)", len(orphans))
	}
}
