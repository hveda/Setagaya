//go:build integration

package main

import (
	"testing"

	"github.com/hveda/Setagaya/v3/internal/config"
	"github.com/hveda/Setagaya/v3/test/dbtest"
)

// TestNewProjectRepository_MySQL covers the mysql wiring branch: open, ping and
// migrate against a real container.
func TestNewProjectRepository_MySQL(t *testing.T) {
	dsn := dbtest.StartMySQLDSN(t)

	repo, err := newRepository(config.DBConfig{Driver: "mysql", DSN: dsn})
	if err != nil {
		t.Fatalf("newRepository(mysql): %v", err)
	}
	if repo == nil {
		t.Fatal("newRepository(mysql) returned nil repo")
	}
}
