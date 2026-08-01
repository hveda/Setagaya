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
		runID, err := repo.StartRun(ctx, execution)
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
		if _, err := repo.StartRun(ctx, execution); err == nil {
			t.Fatal("second StartRun: want ErrRunActive, got nil")
		}
	})

	t.Run("stop clears the active run and allows a new one", func(t *testing.T) {
		repo := newRepo(t)
		first, err := repo.StartRun(ctx, execution)
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
		second, err := repo.StartRun(ctx, execution)
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
		runID, err := repo.StartRun(ctx, execution)
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
}
