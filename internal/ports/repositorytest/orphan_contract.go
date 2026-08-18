package repositorytest

import (
	"context"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/ports"
)

// NewOrphanRepo builds a fresh, empty OrphanRepository for one test.
type NewOrphanRepo func(t *testing.T) ports.OrphanRepository

// RunOrphanRepositoryContract pins the behaviour every OrphanRepository must
// share: a retried Final is one event, rows are execution-scoped, and Deploy's
// clear wipes exactly its own execution's evidence.
func RunOrphanRepositoryContract(t *testing.T, newRepo NewOrphanRepo) {
	t.Helper()
	ctx := context.Background()
	const execution = int64(7)
	code := 3
	at := time.Unix(2000, 0).UTC()

	oc := ports.OrphanCompletion{
		ExecutionID: execution, ScenarioID: 70, ShardIndex: 1,
		ExitCode: &code, FinishedAt: at,
	}

	t.Run("record then list round-trips", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.RecordOrphanCompletion(ctx, oc); err != nil {
			t.Fatalf("RecordOrphanCompletion: %v", err)
		}
		got, err := repo.OrphanCompletions(ctx, execution)
		if err != nil {
			t.Fatalf("OrphanCompletions: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d orphans, want 1", len(got))
		}
		if got[0].ScenarioID != oc.ScenarioID || got[0].ShardIndex != oc.ShardIndex {
			t.Fatalf("orphan = %+v, want scenario %d shard %d", got[0], oc.ScenarioID, oc.ShardIndex)
		}
		if got[0].ExitCode == nil || *got[0].ExitCode != code {
			t.Fatalf("exit code = %v, want %d", got[0].ExitCode, code)
		}
		if !got[0].FinishedAt.Equal(at) {
			t.Fatalf("finished at = %v, want %v", got[0].FinishedAt, at)
		}
	})

	t.Run("a retried final overwrites, not accumulates", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.RecordOrphanCompletion(ctx, oc); err != nil {
			t.Fatalf("record: %v", err)
		}
		retry := oc
		later := at.Add(time.Minute)
		retry.FinishedAt = later
		if err := repo.RecordOrphanCompletion(ctx, retry); err != nil {
			t.Fatalf("re-record: %v", err)
		}
		got, err := repo.OrphanCompletions(ctx, execution)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d orphans after a retry, want 1 (one event)", len(got))
		}
		if !got[0].FinishedAt.Equal(later) {
			t.Fatalf("orphan kept the first attempt's time: %v, want %v", got[0].FinishedAt, later)
		}
	})

	t.Run("a final without an exit code is kept as nil", func(t *testing.T) {
		repo := newRepo(t)
		noCode := ports.OrphanCompletion{
			ExecutionID: execution, ScenarioID: 71, ShardIndex: 0,
			FinishedAt: at,
		}
		if err := repo.RecordOrphanCompletion(ctx, noCode); err != nil {
			t.Fatalf("record: %v", err)
		}
		got, err := repo.OrphanCompletions(ctx, execution)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 || got[0].ExitCode != nil {
			t.Fatalf("orphan = %+v, want one with a nil exit code", got)
		}
	})

	t.Run("rows are execution-scoped", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.RecordOrphanCompletion(ctx, oc); err != nil {
			t.Fatalf("record: %v", err)
		}
		other, err := repo.OrphanCompletions(ctx, 99)
		if err != nil {
			t.Fatalf("list other: %v", err)
		}
		if len(other) != 0 {
			t.Fatalf("execution 99 sees %d orphans, want 0", len(other))
		}
	})

	t.Run("clear drops only this execution's rows", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.RecordOrphanCompletion(ctx, oc); err != nil {
			t.Fatalf("record: %v", err)
		}
		other := oc
		other.ExecutionID = 99
		if err := repo.RecordOrphanCompletion(ctx, other); err != nil {
			t.Fatalf("record other: %v", err)
		}
		if err := repo.ClearOrphanCompletions(ctx, execution); err != nil {
			t.Fatalf("clear: %v", err)
		}
		got, err := repo.OrphanCompletions(ctx, execution)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("execution %d kept %d orphans after clear", execution, len(got))
		}
		kept, err := repo.OrphanCompletions(ctx, 99)
		if err != nil {
			t.Fatalf("list other: %v", err)
		}
		if len(kept) != 1 {
			t.Fatalf("clear leaked into execution 99 (%d rows)", len(kept))
		}
		// Clearing an execution with no orphans is not an error.
		if err := repo.ClearOrphanCompletions(ctx, execution); err != nil {
			t.Fatalf("clear empty: %v", err)
		}
	})
}
