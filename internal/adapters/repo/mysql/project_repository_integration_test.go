//go:build integration

package mysql_test

import (
	"context"
	"database/sql"
	"testing"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

// TestMySQLProjectRepository_Contract runs the shared repository conformance
// suite against a real MySQL container, proving the MySQL adapter is behaviour-
// compatible with the in-memory fake.
func TestMySQLProjectRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)

	repositorytest.RunProjectRepositoryContract(t, func(t *testing.T) ports.ProjectRepository {
		truncate(t, db)
		return mysqladapter.NewProjectRepository(db)
	})
}

// TestMySQLProjectRepository_TenantAndAuditRoundTrip exercises the nullable
// columns (tenant_id, created_by, updated_by) that the contract's project.New
// inputs leave empty.
func TestMySQLProjectRepository_TenantAndAuditRoundTrip(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewProjectRepository(db)
	ctx := context.Background()

	tenant := int64(42)
	id, err := repo.CreateProject(ctx, project.Project{
		Name:      "tenant-scoped",
		Owner:     "team-a",
		SID:       "9",
		TenantID:  &tenant,
		CreatedBy: "okta|abc",
		UpdatedBy: "okta|def",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := repo.GetProject(ctx, id)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.TenantID == nil || *got.TenantID != tenant {
		t.Errorf("TenantID = %v, want %d", got.TenantID, tenant)
	}
	if got.CreatedBy != "okta|abc" || got.UpdatedBy != "okta|def" {
		t.Errorf("audit fields = %q/%q, want okta|abc/okta|def", got.CreatedBy, got.UpdatedBy)
	}
}

// TestMySQLProjectRepository_ErrorsWhenDBClosed drives the DB-error branches by
// closing the connection pool before each call.
func TestMySQLProjectRepository_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewProjectRepository(db)
	ctx := context.Background()

	p := project.Project{Name: "x", Owner: "team-a"}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if _, err := repo.CreateProject(ctx, p); err == nil {
		t.Error("CreateProject on closed db: want error")
	}
	if _, err := repo.GetProject(ctx, 1); err == nil {
		t.Error("GetProject on closed db: want error")
	}
	if _, err := repo.ListProjectsByOwners(ctx, []string{"team-a"}); err == nil {
		t.Error("ListProjectsByOwners on closed db: want error")
	}
	if err := repo.DeleteProject(ctx, 1); err == nil {
		t.Error("DeleteProject on closed db: want error")
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	// StartMySQL already migrated once; a second run must be a clean no-op that
	// exercises the "already applied" skip path.
	db := dbtest.StartMySQL(t)
	if err := mysqladapter.Migrate(context.Background(), db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrate_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if err := mysqladapter.Migrate(context.Background(), db); err == nil {
		t.Error("Migrate on closed db: want error")
	}
}

func truncate(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("TRUNCATE TABLE project"); err != nil {
		t.Fatalf("truncate project: %v", err)
	}
}
