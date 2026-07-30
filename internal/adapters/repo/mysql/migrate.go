package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/heridotlife/Setagaya/migrations"
)

const createSchemaMigrations = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    VARCHAR(255) NOT NULL PRIMARY KEY,
	applied_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) CHARSET=utf8mb4`

// legacyTables are the pre-Honryu table names. Honryu renamed every migration
// file except 0001_project.sql, so against a database migrated by the old set
// the renamed files look unapplied: they would run happily, create empty
// scenario/execution tables beside the populated plan/collection ones, and
// report success -- leaving the API healthy but apparently empty. Refusing is
// the only safe answer, because there is no supported path from that schema:
// Honryu migrates from Shibuya by importing JMX assets into a fresh database.
var legacyTables = []string{
	"plan", "collection", "collection_plan", "plan_data", "plan_test_file",
	"collection_data", "collection_run", "collection_run_history",
	"running_plan", "collection_launch", "collection_launch_history2",
	"v3_tenant", "v3_role_grant",
}

// Migrate applies any embedded migrations not yet recorded in
// schema_migrations, in lexical filename order. It is idempotent: running it
// repeatedly is a no-op once all migrations are applied.
//
// Convention: each migration file contains a single SQL statement, so the app
// connection does not need multiStatements enabled.
func Migrate(ctx context.Context, db *sql.DB) error {
	if err := rejectLegacySchema(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, createSchemaMigrations); err != nil {
		return fmt.Errorf("mysql: ensure schema_migrations: %w", err)
	}

	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return fmt.Errorf("mysql: list migrations: %w", err)
	}
	sort.Strings(files)

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	for _, name := range files {
		if applied[name] {
			continue
		}
		content, readErr := migrations.FS.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("mysql: read migration %s: %w", name, readErr)
		}
		if _, execErr := db.ExecContext(ctx, string(content)); execErr != nil {
			return fmt.Errorf("mysql: apply migration %s: %w", name, execErr)
		}
		if _, recErr := db.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", name); recErr != nil {
			return fmt.Errorf("mysql: record migration %s: %w", name, recErr)
		}
	}
	return nil
}

// rejectLegacySchema fails before anything is created when the database still
// carries pre-Honryu tables, so the operator is left with exactly what they had.
func rejectLegacySchema(ctx context.Context, db *sql.DB) error {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(legacyTables)), ",")
	args := make([]any, len(legacyTables))
	for i, t := range legacyTables {
		args[i] = t
	}

	rows, err := db.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables"+
			" WHERE table_schema = DATABASE() AND table_name IN ("+placeholders+")"+
			" ORDER BY table_name", args...)
	if err != nil {
		return fmt.Errorf("mysql: check for legacy schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var found []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return fmt.Errorf("mysql: scan legacy table name: %w", scanErr)
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mysql: iterate legacy table names: %w", err)
	}
	if len(found) > 0 {
		return fmt.Errorf(
			"mysql: database carries pre-Honryu tables (%s); Honryu requires a fresh database"+
				" -- migrate from Shibuya by importing JMX assets, not by upgrading this schema",
			strings.Join(found, ", "))
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("mysql: load applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if scanErr := rows.Scan(&v); scanErr != nil {
			return nil, fmt.Errorf("mysql: scan migration version: %w", scanErr)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate migration versions: %w", err)
	}
	return applied, nil
}
