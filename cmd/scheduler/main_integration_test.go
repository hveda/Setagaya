//go:build integration

package main

import (
	"testing"

	"github.com/heridotlife/honryu/internal/config"
	"github.com/heridotlife/honryu/test/dbtest"
)

// TestNewRepository_MySQL covers the mysql wiring branch: open and ping
// against a real container. Unlike cmd/api's own newRepository, this one does
// not apply migrations -- cmd/api owns that -- so an unmigrated container is
// exactly the state it expects to find.
func TestNewRepository_MySQL(t *testing.T) {
	dsn := dbtest.StartMySQLDSN(t)

	repo, err := newRepository(config.DBConfig{Driver: "mysql", DSN: dsn}, "default")
	if err != nil {
		t.Fatalf("newRepository(mysql): %v", err)
	}
	if repo == nil {
		t.Fatal("newRepository(mysql) returned nil repo")
	}
}
