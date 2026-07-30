package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/ports"
)

const collectionColumns = "id, name, project_id, csv_split, tenant_id, created_by, updated_by, created_time"

// CreateExecution inserts c and returns its auto-assigned ID.
func (r *Repository) CreateExecution(ctx context.Context, c execution.Execution) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO collection (name, project_id, csv_split, tenant_id, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)",
		c.Name, c.ProjectID, boolToInt(c.CSVSplit), nullInt64(c.TenantID), nullString(c.CreatedBy), nullString(c.UpdatedBy),
	)
	if err != nil {
		return 0, fmt.Errorf("mysql: create collection: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create collection last id: %w", err)
	}
	return id, nil
}

// GetExecution returns the collection with id, or ports.ErrNotFound.
func (r *Repository) GetExecution(ctx context.Context, id int64) (execution.Execution, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+collectionColumns+" FROM collection WHERE id = ?", id)
	c, err := scanCollection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return execution.Execution{}, ports.ErrNotFound
	}
	if err != nil {
		return execution.Execution{}, fmt.Errorf("mysql: get collection: %w", err)
	}
	return c, nil
}

// ListExecutionsByProject returns all collections belonging to projectID.
func (r *Repository) ListExecutionsByProject(ctx context.Context, projectID int64) ([]execution.Execution, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+collectionColumns+" FROM collection WHERE project_id = ?", projectID)
	if err != nil {
		return nil, fmt.Errorf("mysql: list collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []execution.Execution{}
	for rows.Next() {
		c, scanErr := scanCollection(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan collection: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate collections: %w", err)
	}
	return out, nil
}

// DeleteExecution removes the collection with id, or ports.ErrNotFound.
func (r *Repository) DeleteExecution(ctx context.Context, id int64) error {
	return execDelete(ctx, r.db, "DELETE FROM collection WHERE id = ?", id)
}

// AddExecutionFile records a data file for the collection, or
// ports.ErrFileExists on duplicate.
func (r *Repository) AddExecutionFile(ctx context.Context, executionID int64, filename string) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO collection_data (collection_id, filename) VALUES (?, ?)", executionID, filename)
	if isDuplicateKey(err) {
		return ports.ErrFileExists
	}
	if err != nil {
		return fmt.Errorf("mysql: add collection file: %w", err)
	}
	return nil
}

// ExecutionFilesFor returns the collection's data files.
func (r *Repository) ExecutionFilesFor(ctx context.Context, executionID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT filename FROM collection_data WHERE collection_id = ?", executionID)
	if err != nil {
		return nil, fmt.Errorf("mysql: collection files: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, fmt.Errorf("mysql: scan collection file: %w", scanErr)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate collection files: %w", err)
	}
	return out, nil
}

// DeleteExecutionFile removes a data file record, or ports.ErrNotFound.
func (r *Repository) DeleteExecutionFile(ctx context.Context, executionID int64, filename string) error {
	return execDelete(ctx, r.db, "DELETE FROM collection_data WHERE collection_id = ? AND filename = ?", executionID, filename)
}

// StoreLoadProfile replaces the collection's execution plans and updates
// its csv_split flag atomically. Returns ports.ErrNotFound if the collection
// does not exist.
func (r *Repository) StoreLoadProfile(ctx context.Context, executionID int64, csvSplit bool, plans []loadprofile.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mysql: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM collection WHERE id = ?)", executionID).Scan(&exists); err != nil {
		return fmt.Errorf("mysql: check collection: %w", err)
	}
	if !exists {
		return ports.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM collection_plan WHERE collection_id = ?", executionID); err != nil {
		return fmt.Errorf("mysql: clear execution plans: %w", err)
	}
	for _, ep := range plans {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO collection_plan (collection_id, plan_id, concurrency, rampup, duration, engines, csv_split) VALUES (?, ?, ?, ?, ?, ?, ?)",
			executionID, ep.ScenarioID, ep.Concurrency, ep.Rampup, ep.Duration, ep.Engines, boolToInt(ep.CSVSplit),
		); err != nil {
			return fmt.Errorf("mysql: insert execution plan: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE collection SET csv_split = ? WHERE id = ?", boolToInt(csvSplit), executionID); err != nil {
		return fmt.Errorf("mysql: update csv_split: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mysql: commit: %w", err)
	}
	return nil
}

// LoadProfileFor returns the collection's current execution plans. Plan names
// are not persisted, so ExecutionPlan.Name is empty.
func (r *Repository) LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT plan_id, concurrency, rampup, duration, engines, csv_split FROM collection_plan WHERE collection_id = ?", executionID)
	if err != nil {
		return nil, fmt.Errorf("mysql: execution plans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []loadprofile.Entry{}
	for rows.Next() {
		var (
			ep       loadprofile.Entry
			engines  sql.NullInt64
			csvSplit int64
		)
		if scanErr := rows.Scan(&ep.ScenarioID, &ep.Concurrency, &ep.Rampup, &ep.Duration, &engines, &csvSplit); scanErr != nil {
			return nil, fmt.Errorf("mysql: scan execution plan: %w", scanErr)
		}
		ep.Engines = int(engines.Int64)
		ep.CSVSplit = csvSplit != 0
		out = append(out, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate execution plans: %w", err)
	}
	return out, nil
}

func scanCollection(s rowScanner) (execution.Execution, error) {
	var (
		c         execution.Execution
		csvSplit  int64
		tenantID  sql.NullInt64
		createdBy sql.NullString
		updatedBy sql.NullString
	)
	if err := s.Scan(&c.ID, &c.Name, &c.ProjectID, &csvSplit, &tenantID, &createdBy, &updatedBy, &c.CreatedTime); err != nil {
		return execution.Execution{}, err
	}
	c.CSVSplit = csvSplit != 0
	c.CreatedBy = createdBy.String
	c.UpdatedBy = updatedBy.String
	if tenantID.Valid {
		c.TenantID = &tenantID.Int64
	}
	return c, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
