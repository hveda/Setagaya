package repositorytest

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
)

// NewRunRepo builds a fresh, empty RunRepository for one test.
type NewRunRepo func(t *testing.T) ports.RunRepository

// RunRunRepositoryContract pins the behaviour every RunRepository must share.
func RunRunRepositoryContract(t *testing.T, newRepo NewRunRepo) {
	t.Helper()
	ctx := context.Background()
	const execution = int64(42)

	t.Run("start reports active and rejects a second start", func(t *testing.T) {
		repo := newRepo(t)
		runID, err := repo.StartRun(ctx, execution, "")
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		if runID <= 0 {
			t.Fatalf("runID = %d, want > 0", runID)
		}
		got, ok, err := repo.CurrentRun(ctx, execution)
		if err != nil || !ok || got != runID {
			t.Fatalf("CurrentRun = %d,%v,%v; want %d,true,nil", got, ok, err, runID)
		}
		if _, err := repo.StartRun(ctx, execution, ""); err == nil {
			t.Fatal("second StartRun: want ErrRunActive, got nil")
		}
	})

	t.Run("stop clears the active run and allows a new one", func(t *testing.T) {
		repo := newRepo(t)
		first, err := repo.StartRun(ctx, execution, "")
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		if err := repo.StopRun(ctx, execution); err != nil {
			t.Fatalf("StopRun: %v", err)
		}
		if _, ok, _ := repo.CurrentRun(ctx, execution); ok {
			t.Fatal("CurrentRun after stop: want ok=false")
		}
		// Stopping again is a no-op.
		if err := repo.StopRun(ctx, execution); err != nil {
			t.Fatalf("StopRun (no active): %v", err)
		}
		second, err := repo.StartRun(ctx, execution, "")
		if err != nil {
			t.Fatalf("re-StartRun: %v", err)
		}
		if second == first {
			t.Fatalf("re-StartRun reused run id %d", second)
		}
	})

	// A report needs to know when its run started, and nothing else keeps that
	// once the run has been superseded or stopped.
	t.Run("run history records the start and, once stopped, the end", func(t *testing.T) {
		repo := newRepo(t)
		runID, err := repo.StartRun(ctx, execution, "")
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}

		rec, err := repo.RunHistory(ctx, runID)
		if err != nil {
			t.Fatalf("RunHistory: %v", err)
		}
		if rec.RunID != runID || rec.ExecutionID != execution {
			t.Fatalf("RunHistory identity = %+v", rec)
		}
		if rec.StartedTime.IsZero() {
			t.Error("StartedTime is zero")
		}
		if rec.EndTime != nil {
			t.Errorf("EndTime = %v before the run stopped, want nil", rec.EndTime)
		}

		if err := repo.StopRun(ctx, execution); err != nil {
			t.Fatalf("StopRun: %v", err)
		}
		rec, err = repo.RunHistory(ctx, runID)
		if err != nil {
			t.Fatalf("RunHistory after stop: %v", err)
		}
		if rec.EndTime == nil {
			t.Error("EndTime is nil after the run stopped")
		}

		if _, err := repo.RunHistory(ctx, 999999); !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("RunHistory(unknown) = %v, want ErrNotFound", err)
		}
	})

	// The correlation id a run was started with is the one its report will
	// surface, and the history row is where it lives on: reading the
	// execution's pending value instead would show a later deploy's id.
	t.Run("run history keeps the correlation id it was started with", func(t *testing.T) {
		repo := newRepo(t)
		const want = "4bf92f3577b34da6a3ce929d0e0e4736"
		runID, err := repo.StartRun(ctx, execution, want)
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		rec, err := repo.RunHistory(ctx, runID)
		if err != nil {
			t.Fatalf("RunHistory: %v", err)
		}
		if rec.CorrelationID != want {
			t.Fatalf("RunHistory correlation = %q, want %q", rec.CorrelationID, want)
		}

		// An empty start (a deploy that minted nothing) stays empty, and a
		// stop does not rewrite it.
		if err := repo.StopRun(ctx, execution); err != nil {
			t.Fatalf("StopRun: %v", err)
		}
		rec, err = repo.RunHistory(ctx, runID)
		if err != nil {
			t.Fatalf("RunHistory after stop: %v", err)
		}
		if rec.CorrelationID != want {
			t.Fatalf("RunHistory correlation after stop = %q, want %q", rec.CorrelationID, want)
		}
		empty, err := repo.StartRun(ctx, execution, "")
		if err != nil {
			t.Fatalf("StartRun(empty): %v", err)
		}
		rec, err = repo.RunHistory(ctx, empty)
		if err != nil {
			t.Fatalf("RunHistory(empty): %v", err)
		}
		if rec.CorrelationID != "" {
			t.Fatalf("RunHistory correlation for an empty start = %q, want empty", rec.CorrelationID)
		}
	})

	t.Run("running scenarios are tracked per execution and idempotent", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.MarkScenarioRunning(ctx, execution, 1); err != nil {
			t.Fatalf("MarkScenarioRunning: %v", err)
		}
		if err := repo.MarkScenarioRunning(ctx, execution, 1); err != nil {
			t.Fatalf("MarkScenarioRunning (dup): %v", err)
		}
		if err := repo.MarkScenarioRunning(ctx, execution, 2); err != nil {
			t.Fatalf("MarkScenarioRunning: %v", err)
		}

		byColl, err := repo.RunningScenariosByExecution(ctx, execution)
		if err != nil {
			t.Fatalf("RunningScenariosByExecution: %v", err)
		}
		if len(byColl) != 2 {
			t.Fatalf("running scenarios = %d, want 2 (idempotent)", len(byColl))
		}

		all, err := repo.RunningScenarios(ctx)
		if err != nil {
			t.Fatalf("RunningScenarios: %v", err)
		}
		if len(all) < 2 {
			t.Fatalf("RunningScenarios = %d, want >= 2", len(all))
		}

		if err := repo.ClearScenarioRunning(ctx, execution, 1); err != nil {
			t.Fatalf("ClearScenarioRunning: %v", err)
		}
		byColl, _ = repo.RunningScenariosByExecution(ctx, execution)
		if len(byColl) != 1 || byColl[0].ScenarioID != 2 {
			t.Fatalf("after clear = %+v, want only scenario 2", byColl)
		}
		// Clearing a missing marker is a no-op.
		if err := repo.ClearScenarioRunning(ctx, execution, 999); err != nil {
			t.Fatalf("ClearScenarioRunning (missing): %v", err)
		}
	})

	// OpenRuns is the reconciliation scan: every execution's active run with
	// its start time, gone once the run is stopped.
	t.Run("open runs list active runs until stopped", func(t *testing.T) {
		repo := newRepo(t)
		runID, err := repo.StartRun(ctx, execution, "")
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		got, err := repo.OpenRuns(ctx)
		if err != nil {
			t.Fatalf("OpenRuns: %v", err)
		}
		if len(got) != 1 || got[0].ExecutionID != execution || got[0].RunID != runID {
			t.Fatalf("OpenRuns = %+v, want execution %d run %d", got, execution, runID)
		}
		if got[0].StartedTime.IsZero() {
			t.Fatalf("OpenRuns missing the run's start time: %+v", got[0])
		}

		if err := repo.StopRun(ctx, execution); err != nil {
			t.Fatalf("StopRun: %v", err)
		}
		after, err := repo.OpenRuns(ctx)
		if err != nil {
			t.Fatalf("OpenRuns after stop: %v", err)
		}
		if len(after) != 0 {
			t.Fatalf("OpenRuns after stop = %+v, want none", after)
		}
	})
}
