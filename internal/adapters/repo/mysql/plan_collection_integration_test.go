//go:build integration

package mysql_test

import (
	"context"
	"database/sql"
	"testing"

	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/internal/domain/collection"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports/repositorytest"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

func TestMySQLPlanRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunPlanRepositoryContract(t, func(t *testing.T) repositorytest.Repository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

func TestMySQLCollectionRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunCollectionRepositoryContract(t, func(t *testing.T) repositorytest.Repository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// TestMySQLPlan_TenantAndAuditRoundTrip covers the nullable plan columns.
func TestMySQLPlan_TenantAndAuditRoundTrip(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	tenant := int64(7)
	id, err := repo.CreatePlan(ctx, scenario.Scenario{
		Name: "scoped", ProjectID: 3, TenantID: &tenant, CreatedBy: "okta|a", UpdatedBy: "okta|b",
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	got, err := repo.GetPlan(ctx, id)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if got.TenantID == nil || *got.TenantID != tenant || got.CreatedBy != "okta|a" || got.UpdatedBy != "okta|b" {
		t.Fatalf("plan round trip = %+v", got)
	}
}

// TestMySQLCollection_TenantAndCSVRoundTrip covers the nullable collection
// columns and the csv_split flag on create.
func TestMySQLCollection_TenantAndCSVRoundTrip(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	tenant := int64(9)
	id, err := repo.CreateCollection(ctx, collection.Collection{
		Name: "scoped", ProjectID: 3, CSVSplit: true, TenantID: &tenant, CreatedBy: "okta|c", UpdatedBy: "okta|d",
	})
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	got, err := repo.GetCollection(ctx, id)
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if got.TenantID == nil || *got.TenantID != tenant || !got.CSVSplit || got.CreatedBy != "okta|c" {
		t.Fatalf("collection round trip = %+v", got)
	}
}

// TestMySQLPlanCollection_ErrorsWhenDBClosed drives the DB-error branches of
// every plan and collection method by closing the pool first.
func TestMySQLPlanCollection_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	ops := map[string]func() error{
		"CreatePlan":         func() error { _, e := repo.CreatePlan(ctx, scenario.Scenario{Name: "x", ProjectID: 1}); return e },
		"GetPlan":            func() error { _, e := repo.GetPlan(ctx, 1); return e },
		"ListPlansByProject": func() error { _, e := repo.ListPlansByProject(ctx, 1); return e },
		"DeletePlan":         func() error { return repo.DeletePlan(ctx, 1) },
		"AddPlanFile":        func() error { return repo.AddPlanFile(ctx, 1, "a", false) },
		"PlanFilesFor":       func() error { _, e := repo.PlanFilesFor(ctx, 1); return e },
		"DeletePlanFile":     func() error { return repo.DeletePlanFile(ctx, 1, "a", false) },
		"PlanInUse":          func() error { _, e := repo.PlanInUse(ctx, 1); return e },
		"CreateCollection": func() error {
			_, e := repo.CreateCollection(ctx, collection.Collection{Name: "x", ProjectID: 1})
			return e
		},
		"GetCollection":            func() error { _, e := repo.GetCollection(ctx, 1); return e },
		"ListCollectionsByProject": func() error { _, e := repo.ListCollectionsByProject(ctx, 1); return e },
		"DeleteCollection":         func() error { return repo.DeleteCollection(ctx, 1) },
		"AddCollectionFile":        func() error { return repo.AddCollectionFile(ctx, 1, "a") },
		"CollectionFilesFor":       func() error { _, e := repo.CollectionFilesFor(ctx, 1); return e },
		"DeleteCollectionFile":     func() error { return repo.DeleteCollectionFile(ctx, 1, "a") },
		"StoreExecutionCollection": func() error { return repo.StoreExecutionCollection(ctx, 1, false, nil) },
		"ExecutionPlansFor":        func() error { _, e := repo.ExecutionPlansFor(ctx, 1); return e },
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
		"project", "plan", "collection", "collection_plan",
		"plan_data", "plan_test_file", "collection_data",
		"collection_run", "collection_run_history", "running_plan",
		"collection_launch", "collection_launch_history2",
		"v3_tenant", "v3_role_grant",
	} {
		if _, err := db.Exec("TRUNCATE TABLE " + table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}
