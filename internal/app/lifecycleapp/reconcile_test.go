package lifecycleapp_test

import (
	"context"
	"testing"
	"time"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// reconcileEnv is a lifecycle env whose metrics hook is the real metricsapp
// service over fakes, so a reconciled run writes a real report.
func reconcileEnv(t *testing.T, engines ...int) (*env, *fake.ReportStore) {
	t.Helper()
	e := setup(t, false, engines...)
	reports := fake.NewReportStore()
	collector := metricsapp.NewService(e.store, fake.NewMetricsSink(), membus.New(), fake.NewReportProgress(), reports)
	e.svc.WithMetrics(collector)
	return e, reports
}

// orphanFor records one shard's orphaned Final straight into the store -- the
// state Ingest leaves behind when a Final arrives with no open run.
func orphanFor(t *testing.T, e *env, scenarioID int64, shard, exitCode int) {
	t.Helper()
	oc := ports.OrphanCompletion{
		ExecutionID: e.executionID, ScenarioID: scenarioID, ShardIndex: shard,
		FinishedAt: time.Unix(1000, 0),
	}
	if exitCode >= 0 {
		oc.ExitCode = &exitCode
	}
	if err := e.store.RecordOrphanCompletion(context.Background(), oc); err != nil {
		t.Fatalf("RecordOrphanCompletion: %v", err)
	}
}

// The stranded run a reconciliation pass exists for: the run stood open, the
// engines ran out their hold, their Finals arrived orphaned, and the report
// never happened. Reconcile closes it with the orphans' own evidence.
func TestReconcile_FinalizesStrandedRunFromOrphanEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, reports := reconcileEnv(t, 1)

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	runID, ok, err := e.store.CurrentRun(ctx, e.executionID)
	if err != nil || !ok {
		t.Fatalf("CurrentRun: %d, %v", runID, err)
	}

	// The engine finished while the run stood open: exit 3 is bzt's
	// criteria-failed code, and it must win over the abort baseline.
	orphanFor(t, e, e.planIDs[0], 0, 3)

	if err := e.svc.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	rep, err := reports.GetReport(ctx, runID)
	if err != nil {
		t.Fatalf("GetReport after reconcile: %v", err)
	}
	if rep.Outcome != "failed" {
		t.Fatalf("outcome = %q, want failed (the orphaned shard's exit 3)", rep.Outcome)
	}

	// The run is closed, not just reported: a redeploy + trigger must work.
	if _, running, _ := e.store.CurrentRun(ctx, e.executionID); running {
		t.Fatal("reconcile left the stranded run open")
	}
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("re-Deploy after reconcile: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger after redeploy: %v", err)
	}
}

// A run with no orphan evidence may genuinely still be running: Reconcile
// must leave it -- and its empty report table -- alone.
func TestReconcile_LeavesLiveRunsAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, reports := reconcileEnv(t, 2)
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	if err := e.svc.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, running, _ := e.store.CurrentRun(ctx, e.executionID); !running {
		t.Fatal("Reconcile closed a run with no evidence against it")
	}
	if reps, _ := reports.ListReports(ctx, e.executionID, 0); len(reps) != 0 {
		t.Fatalf("Reconcile wrote %d reports for a live run", len(reps))
	}
}

// Partial orphan coverage is a pod dying mid-run, not the pool finishing --
// the run's own progress handles that; Reconcile waits for full coverage.
func TestReconcile_IgnoresPartialOrphanCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, reports := reconcileEnv(t, 2) // two shards planned
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	orphanFor(t, e, e.planIDs[0], 0, 0) // only shard 0's Final orphaned

	if err := e.svc.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if reps, _ := reports.ListReports(ctx, e.executionID, 0); len(reps) != 0 {
		t.Fatalf("Reconcile finalized on partial evidence: %+v", reps)
	}
}

// Reconcile is a periodic pass: running it twice must not double-report or
// error (SaveReport's first-wins plus the closed run row make it a no-op).
func TestReconcile_IsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e, reports := reconcileEnv(t, 1)
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	orphanFor(t, e, e.planIDs[0], 0, 0)

	for range 2 {
		if err := e.svc.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	}
	reps, err := reports.ListReports(ctx, e.executionID, 0)
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(reps) != 1 {
		t.Fatalf("reports = %d, want exactly 1", len(reps))
	}
}
