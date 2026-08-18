//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLUsageRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunUsageRepositoryContract(t, func(t *testing.T) ports.UsageRepository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// TestMySQLUsage_ErrorsWhenDBClosed drives the DB-error branches of the usage
// repo.
func TestMySQLUsage_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	ops := map[string]func() error{
		"StartLaunch":   func() error { return repo.StartLaunch(ctx, 1, "o", 1, 1) },
		"FinishLaunch":  func() error { return repo.FinishLaunch(ctx, 1, 1) },
		"LaunchHistory": func() error { _, e := repo.LaunchHistory(ctx, time.Now().Add(-time.Hour), time.Now()); return e },
	}
	for name, op := range ops {
		if err := op(); err == nil {
			t.Errorf("%s on closed db: want error, got nil", name)
		}
	}
}
