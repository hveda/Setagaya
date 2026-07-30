//go:build integration

package mysql_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

// TestMigrate_RejectsLegacySchema guards the Shibuya -> Honryu cutover.
//
// Migrations are tracked by filename. The Honryu baseline renamed every file
// except 0001_project.sql, so against a database migrated by the old set the
// renamed files look unapplied: they would run, create empty scenario/execution
// tables beside the populated plan/collection ones, and report success. The API
// would then start healthy with every project appearing to have no scenarios.
// Migrate must refuse instead.
func TestMigrate_RejectsLegacySchema(t *testing.T) {
	ctx := context.Background()
	dsn := dbtest.StartMySQLDSN(t)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// A database left behind by the pre-Honryu schema.
	if _, err := db.ExecContext(ctx, `CREATE TABLE collection (
		id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(100) NOT NULL
	) CHARSET=utf8mb4`); err != nil {
		t.Fatalf("seed legacy table: %v", err)
	}

	err = mysqladapter.Migrate(ctx, db)
	if err == nil {
		t.Fatal("Migrate accepted a legacy database; want refusal")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Errorf("error %q does not name the legacy table", err)
	}

	// The refusal must come before any Honryu table is created, so the operator
	// is left with exactly what they had.
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'scenario'",
	).Scan(&n); err != nil {
		t.Fatalf("count scenario table: %v", err)
	}
	if n != 0 {
		t.Error("Migrate created Honryu tables before refusing")
	}
}
