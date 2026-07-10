// Package mysql is the MySQL/MariaDB implementation of the repository ports.
// It maps domain aggregates onto the shared v2/v3 schema. Behaviour is pinned
// by the shared suite in internal/ports/repositorytest, so it stays
// interchangeable with the in-memory fake.
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// ProjectRepository persists projects in MySQL.
type ProjectRepository struct {
	db *sql.DB
}

// NewProjectRepository returns a ProjectRepository backed by db.
func NewProjectRepository(db *sql.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

var _ ports.ProjectRepository = (*ProjectRepository)(nil)

const projectColumns = "id, name, owner, sid, tenant_id, created_by, updated_by, created_time"

// CreateProject inserts p and returns its auto-assigned ID.
func (r *ProjectRepository) CreateProject(ctx context.Context, p project.Project) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO project (name, owner, sid, tenant_id, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)",
		p.Name, p.Owner, nullString(p.SID), nullInt64(p.TenantID), nullString(p.CreatedBy), nullString(p.UpdatedBy),
	)
	if err != nil {
		return 0, fmt.Errorf("mysql: create project: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create project last id: %w", err)
	}
	return id, nil
}

// GetProject returns the project with id, or ports.ErrNotFound.
func (r *ProjectRepository) GetProject(ctx context.Context, id int64) (project.Project, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+projectColumns+" FROM project WHERE id = ?", id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return project.Project{}, ports.ErrNotFound
	}
	if err != nil {
		return project.Project{}, fmt.Errorf("mysql: get project: %w", err)
	}
	return p, nil
}

// ListProjectsByOwners returns all projects owned by any of owners.
func (r *ProjectRepository) ListProjectsByOwners(ctx context.Context, owners []string) ([]project.Project, error) {
	out := []project.Project{}
	if len(owners) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(owners))
	args := make([]any, len(owners))
	for i, o := range owners {
		placeholders[i] = "?"
		args[i] = o
	}
	// #nosec G201 -- placeholders are fixed "?" tokens; owners are bound params.
	query := fmt.Sprintf("SELECT %s FROM project WHERE owner IN (%s)", projectColumns, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		p, scanErr := scanProject(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan project: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate projects: %w", err)
	}
	return out, nil
}

// DeleteProject removes the project with id, or returns ports.ErrNotFound.
func (r *ProjectRepository) DeleteProject(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM project WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("mysql: delete project: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mysql: delete project rows: %w", err)
	}
	if affected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// rowScanner abstracts *sql.Row and *sql.Rows for a shared scan.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(s rowScanner) (project.Project, error) {
	var (
		p         project.Project
		sid       sql.NullString
		tenantID  sql.NullInt64
		createdBy sql.NullString
		updatedBy sql.NullString
	)
	if err := s.Scan(&p.ID, &p.Name, &p.Owner, &sid, &tenantID, &createdBy, &updatedBy, &p.CreatedTime); err != nil {
		return project.Project{}, err
	}
	p.SID = sid.String
	p.CreatedBy = createdBy.String
	p.UpdatedBy = updatedBy.String
	if tenantID.Valid {
		p.TenantID = &tenantID.Int64
	}
	return p, nil
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
