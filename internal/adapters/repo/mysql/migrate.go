package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"

	"github.com/heridotlife/Setagaya/migrations"
)

const createSchemaMigrations = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    VARCHAR(255) NOT NULL PRIMARY KEY,
	applied_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) CHARSET=utf8mb4`

// Migrate applies any embedded migrations not yet recorded in
// schema_migrations, in lexical filename order. It is idempotent: running it
// repeatedly is a no-op once all migrations are applied.
//
// Convention: each migration file contains a single SQL statement, so the app
// connection does not need multiStatements enabled.
func Migrate(ctx context.Context, db *sql.DB) error {
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
