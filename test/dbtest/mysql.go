//go:build integration || e2e

// Package dbtest provides test helpers that spin up a real MySQL instance via
// testcontainers and apply the v3 migrations. It is only compiled for the
// integration and e2e build tags so the default build stays free of Docker
// dependencies.
package dbtest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
)

// StartMySQLDSN launches a MySQL container and returns a ready connection
// string. Migrations are NOT applied — the caller is responsible for them.
// The container is torn down automatically via t.Cleanup.
func StartMySQLDSN(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("honryu"),
		tcmysql.WithUsername("honryu"),
		tcmysql.WithPassword("secret"),
	)
	if err != nil {
		t.Fatalf("dbtest: start mysql container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("dbtest: connection string: %v", err)
	}
	return dsn
}

// StartMySQL launches a MySQL container, opens a connection, applies all
// migrations, and returns the ready *sql.DB. Container and connection are torn
// down automatically via t.Cleanup.
func StartMySQL(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	dsn := StartMySQLDSN(t)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("dbtest: open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := ping(ctx, db); err != nil {
		t.Fatalf("dbtest: mysql not ready: %v", err)
	}
	if err := mysqladapter.Migrate(ctx, db); err != nil {
		t.Fatalf("dbtest: migrate: %v", err)
	}
	return db
}

func ping(ctx context.Context, db *sql.DB) error {
	deadline := time.Now().Add(30 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if err = db.PingContext(ctx); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
}
