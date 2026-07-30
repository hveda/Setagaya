//go:build integration

package mysql_test

import (
	"context"
	"database/sql"
	"testing"

	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports/repositorytest"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

func TestMySQLScenarioRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunScenarioRepositoryContract(t, func(t *testing.T) repositorytest.Repository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

func TestMySQLExecutionRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunExecutionRepositoryContract(t, func(t *testing.T) repositorytest.Repository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// TestMySQLScenario_TenantAndAuditRoundTrip covers the nullable scenario columns.
func TestMySQLScenario_TenantAndAuditRoundTrip(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	tenant := int64(7)
	id, err := repo.CreateScenario(ctx, scenario.Scenario{
		Name: "scoped", ProjectID: 3, TenantID: &tenant, CreatedBy: "okta|a", UpdatedBy: "okta|b",
	})
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	got, err := repo.GetScenario(ctx, id)
	if err != nil {
		t.Fatalf("GetScenario: %v", err)
	}
	if got.TenantID == nil || *got.TenantID != tenant || got.CreatedBy != "okta|a" || got.UpdatedBy != "okta|b" {
		t.Fatalf("scenario round trip = %+v", got)
	}
}

// TestMySQLExecution_TenantAndCSVRoundTrip covers the nullable execution
// columns and the csv_split flag on create.
func TestMySQLExecution_TenantAndCSVRoundTrip(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	tenant := int64(9)
	id, err := repo.CreateExecution(ctx, execution.Execution{
		Name: "scoped", ProjectID: 3, CSVSplit: true, TenantID: &tenant, CreatedBy: "okta|c", UpdatedBy: "okta|d",
	})
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	got, err := repo.GetExecution(ctx, id)
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}
	if got.TenantID == nil || *got.TenantID != tenant || !got.CSVSplit || got.CreatedBy != "okta|c" {
		t.Fatalf("execution round trip = %+v", got)
	}
}

// TestMySQLScenarioExecution_ErrorsWhenDBClosed drives the DB-error branches of
// every scenario and execution method by closing the pool first.
func TestMySQLScenarioExecution_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	ops := map[string]func() error{
		"CreateScenario":         func() error { _, e := repo.CreateScenario(ctx, scenario.Scenario{Name: "x", ProjectID: 1}); return e },
		"GetScenario":            func() error { _, e := repo.GetScenario(ctx, 1); return e },
		"ListScenariosByProject": func() error { _, e := repo.ListScenariosByProject(ctx, 1); return e },
		"DeleteScenario":         func() error { return repo.DeleteScenario(ctx, 1) },
		"AddScenarioFile":        func() error { return repo.AddScenarioFile(ctx, 1, "a", false) },
		"ScenarioFilesFor":       func() error { _, e := repo.ScenarioFilesFor(ctx, 1); return e },
		"DeleteScenarioFile":     func() error { return repo.DeleteScenarioFile(ctx, 1, "a", false) },
		"ScenarioInUse":          func() error { _, e := repo.ScenarioInUse(ctx, 1); return e },
		"CreateExecution": func() error {
			_, e := repo.CreateExecution(ctx, execution.Execution{Name: "x", ProjectID: 1})
			return e
		},
		"GetExecution":            func() error { _, e := repo.GetExecution(ctx, 1); return e },
		"ListExecutionsByProject": func() error { _, e := repo.ListExecutionsByProject(ctx, 1); return e },
		"DeleteExecution":         func() error { return repo.DeleteExecution(ctx, 1) },
		"AddExecutionFile":        func() error { return repo.AddExecutionFile(ctx, 1, "a") },
		"ExecutionFilesFor":       func() error { _, e := repo.ExecutionFilesFor(ctx, 1); return e },
		"DeleteExecutionFile":     func() error { return repo.DeleteExecutionFile(ctx, 1, "a") },
		"StoreLoadProfile":        func() error { return repo.StoreLoadProfile(ctx, 1, false, nil) },
		"LoadProfileFor":          func() error { _, e := repo.LoadProfileFor(ctx, 1); return e },
	}
	for name, op := range ops {
		if err := op(); err == nil {
			t.Errorf("%s on closed db: want error, got nil", name)
		}
	}
}

func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, table := range []string{
		"project", "scenario", "execution", "execution_scenario",
		"scenario_data", "scenario_test_file", "execution_data",
		"execution_run", "execution_run_history", "running_scenario",
		"execution_launch", "execution_launch_history",
		"tenant", "role_grant",
	} {
		if _, err := db.Exec("TRUNCATE TABLE " + table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}
