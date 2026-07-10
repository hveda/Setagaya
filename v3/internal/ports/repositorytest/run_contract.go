package repositorytest

import (
	"context"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// NewRunRepo builds a fresh, empty RunRepository for one test.
type NewRunRepo func(t *testing.T) ports.RunRepository

// RunRunRepositoryContract pins the behaviour every RunRepository must share.
func RunRunRepositoryContract(t *testing.T, newRepo NewRunRepo) {
	t.Helper()
	ctx := context.Background()
	const collection = int64(42)

	t.Run("start reports active and rejects a second start", func(t *testing.T) {
		repo := newRepo(t)
		runID, err := repo.StartRun(ctx, collection)
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		if runID <= 0 {
			t.Fatalf("runID = %d, want > 0", runID)
		}
		got, ok, err := repo.CurrentRun(ctx, collection)
		if err != nil || !ok || got != runID {
			t.Fatalf("CurrentRun = %d,%v,%v; want %d,true,nil", got, ok, err, runID)
		}
		if _, err := repo.StartRun(ctx, collection); err == nil {
			t.Fatal("second StartRun: want ErrRunActive, got nil")
		}
	})

	t.Run("stop clears the active run and allows a new one", func(t *testing.T) {
		repo := newRepo(t)
		first, err := repo.StartRun(ctx, collection)
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		if err := repo.StopRun(ctx, collection); err != nil {
			t.Fatalf("StopRun: %v", err)
		}
		if _, ok, _ := repo.CurrentRun(ctx, collection); ok {
			t.Fatal("CurrentRun after stop: want ok=false")
		}
		// Stopping again is a no-op.
		if err := repo.StopRun(ctx, collection); err != nil {
			t.Fatalf("StopRun (no active): %v", err)
		}
		second, err := repo.StartRun(ctx, collection)
		if err != nil {
			t.Fatalf("re-StartRun: %v", err)
		}
		if second == first {
			t.Fatalf("re-StartRun reused run id %d", second)
		}
	})

	t.Run("running plans are tracked per collection and idempotent", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.MarkPlanRunning(ctx, collection, 1); err != nil {
			t.Fatalf("MarkPlanRunning: %v", err)
		}
		if err := repo.MarkPlanRunning(ctx, collection, 1); err != nil {
			t.Fatalf("MarkPlanRunning (dup): %v", err)
		}
		if err := repo.MarkPlanRunning(ctx, collection, 2); err != nil {
			t.Fatalf("MarkPlanRunning: %v", err)
		}

		byColl, err := repo.RunningPlansByCollection(ctx, collection)
		if err != nil {
			t.Fatalf("RunningPlansByCollection: %v", err)
		}
		if len(byColl) != 2 {
			t.Fatalf("running plans = %d, want 2 (idempotent)", len(byColl))
		}

		all, err := repo.RunningPlans(ctx)
		if err != nil {
			t.Fatalf("RunningPlans: %v", err)
		}
		if len(all) < 2 {
			t.Fatalf("RunningPlans = %d, want >= 2", len(all))
		}

		if err := repo.ClearPlanRunning(ctx, collection, 1); err != nil {
			t.Fatalf("ClearPlanRunning: %v", err)
		}
		byColl, _ = repo.RunningPlansByCollection(ctx, collection)
		if len(byColl) != 1 || byColl[0].PlanID != 2 {
			t.Fatalf("after clear = %+v, want only plan 2", byColl)
		}
		// Clearing a missing marker is a no-op.
		if err := repo.ClearPlanRunning(ctx, collection, 999); err != nil {
			t.Fatalf("ClearPlanRunning (missing): %v", err)
		}
	})
}
