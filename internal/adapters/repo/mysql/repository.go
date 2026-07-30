// Package mysql is the MySQL/MariaDB implementation of the repository ports.
// A single Repository implements every repository interface over one *sql.DB,
// mapping domain aggregates onto the shared v2/v3 schema. Behaviour is pinned
// by the shared suites in internal/ports/repositorytest, so it stays
// interchangeable with the in-memory fake.
package mysql

import (
	"database/sql"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// defaultDeployContext scopes running_plan rows when no context is configured.
const defaultDeployContext = "default"

// Repository implements ProjectRepository, ScenarioRepository,
// ExecutionRepository and RunRepository over a single MySQL connection pool.
type Repository struct {
	db            *sql.DB
	deployContext string
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, deployContext: defaultDeployContext}
}

// WithContext sets the deployment context recorded on running_plan rows and
// used to scope RunningScenarios queries (mirrors v2's config.SC.Context). An empty
// value is ignored. Returns the receiver for chaining.
func (r *Repository) WithContext(deployContext string) *Repository {
	if deployContext != "" {
		r.deployContext = deployContext
	}
	return r
}

// NewProjectRepository is a convenience constructor for callers that only need
// the project repository surface.
func NewProjectRepository(db *sql.DB) *Repository {
	return NewRepository(db)
}

var (
	_ ports.ProjectRepository   = (*Repository)(nil)
	_ ports.ScenarioRepository  = (*Repository)(nil)
	_ ports.ExecutionRepository = (*Repository)(nil)
	_ ports.RunRepository       = (*Repository)(nil)
	_ ports.UsageRepository     = (*Repository)(nil)
)

// rowScanner abstracts *sql.Row and *sql.Rows for a shared scan.
type rowScanner interface {
	Scan(dest ...any) error
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
