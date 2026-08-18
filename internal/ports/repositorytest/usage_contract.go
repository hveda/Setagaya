package repositorytest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/ports"
)

// NewUsageRepo builds a fresh, empty UsageRepository for one test.
type NewUsageRepo func(t *testing.T) ports.UsageRepository

// RunUsageRepositoryContract pins launch tracking and history behaviour.
func RunUsageRepositoryContract(t *testing.T, newRepo NewUsageRepo) {
	t.Helper()
	ctx := context.Background()
	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)

	t.Run("start, guard, finish, history", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.StartLaunch(ctx, 1, "alice", 4, 40); err != nil {
			t.Fatalf("StartLaunch: %v", err)
		}
		if err := repo.StartLaunch(ctx, 1, "alice", 4, 40); !errors.Is(err, ports.ErrLaunchActive) {
			t.Fatalf("second StartLaunch = %v, want ErrLaunchActive", err)
		}
		if err := repo.FinishLaunch(ctx, 1, 40); err != nil {
			t.Fatalf("FinishLaunch: %v", err)
		}

		hist, err := repo.LaunchHistory(ctx, from, to)
		if err != nil {
			t.Fatalf("LaunchHistory: %v", err)
		}
		if len(hist) != 1 {
			t.Fatalf("history = %d rows, want 1", len(hist))
		}
		got := hist[0]
		if got.ExecutionID != 1 || got.Owner != "alice" || got.Engines != 4 || got.VU != 40 || got.EndTime == nil {
			t.Fatalf("history row = %+v", got)
		}

		// An execution may be launched again once finished, and finishing with
		// nothing open is a no-op.
		if err := repo.StartLaunch(ctx, 1, "alice", 4, 40); err != nil {
			t.Fatalf("re-StartLaunch: %v", err)
		}
		if err := repo.FinishLaunch(ctx, 1, 40); err != nil {
			t.Fatalf("re-FinishLaunch: %v", err)
		}
		if err := repo.FinishLaunch(ctx, 1, 40); err != nil {
			t.Fatalf("FinishLaunch (no open): %v", err)
		}
	})

	t.Run("open launches are excluded from history", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.StartLaunch(ctx, 2, "bob", 2, 20); err != nil {
			t.Fatalf("StartLaunch: %v", err)
		}
		hist, err := repo.LaunchHistory(ctx, from, to)
		if err != nil {
			t.Fatalf("LaunchHistory: %v", err)
		}
		for _, h := range hist {
			if h.ExecutionID == 2 {
				t.Fatalf("open launch leaked into history: %+v", h)
			}
		}
	})
}
