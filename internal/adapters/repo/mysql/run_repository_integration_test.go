//go:build integration

package mysql_test

import (
	"context"
	"testing"

	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/repositorytest"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

func TestMySQLRunRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunRunRepositoryContract(t, func(t *testing.T) ports.RunRepository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// TestMySQLRunRepository_ContextScoping verifies RunningPlans is filtered by the
// configured deployment context while RunningPlansByCollection is not.
func TestMySQLRunRepository_ContextScoping(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	ctx := context.Background()

	repoA := mysqladapter.NewRepository(db).WithContext("ctx-a")
	repoB := mysqladapter.NewRepository(db).WithContext("ctx-b")

	if err := repoA.MarkPlanRunning(ctx, 1, 10); err != nil {
		t.Fatalf("markA: %v", err)
	}
	if err := repoB.MarkPlanRunning(ctx, 2, 20); err != nil {
		t.Fatalf("markB: %v", err)
	}

	aPlans, err := repoA.RunningPlans(ctx)
	if err != nil {
		t.Fatalf("RunningPlans A: %v", err)
	}
	if len(aPlans) != 1 || aPlans[0].CollectionID != 1 {
		t.Fatalf("context A running plans = %+v, want only collection 1", aPlans)
	}
}

// TestMySQLRun_ErrorsWhenDBClosed drives the DB-error branches of the run repo.
func TestMySQLRun_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	ops := map[string]func() error{
		"StartRun":                 func() error { _, e := repo.StartRun(ctx, 1); return e },
		"CurrentRun":               func() error { _, _, e := repo.CurrentRun(ctx, 1); return e },
		"StopRun":                  func() error { return repo.StopRun(ctx, 1) },
		"MarkPlanRunning":          func() error { return repo.MarkPlanRunning(ctx, 1, 2) },
		"ClearPlanRunning":         func() error { return repo.ClearPlanRunning(ctx, 1, 2) },
		"RunningPlans":             func() error { _, e := repo.RunningPlans(ctx); return e },
		"RunningPlansByCollection": func() error { _, e := repo.RunningPlansByCollection(ctx, 1); return e },
	}
	for name, op := range ops {
		if err := op(); err == nil {
			t.Errorf("%s on closed db: want error, got nil", name)
		}
	}
}
