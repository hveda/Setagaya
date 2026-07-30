package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/ports"
)

const executionColumns = "id, name, project_id, csv_split, tenant_id, created_by, updated_by, created_time"

// CreateExecution inserts c and returns its auto-assigned ID.
func (r *Repository) CreateExecution(ctx context.Context, c execution.Execution) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO execution (name, project_id, csv_split, tenant_id, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)",
		c.Name, c.ProjectID, boolToInt(c.CSVSplit), nullInt64(c.TenantID), nullString(c.CreatedBy), nullString(c.UpdatedBy),
	)
	if err != nil {
		return 0, fmt.Errorf("mysql: create execution: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create execution last id: %w", err)
	}
	return id, nil
}

// GetExecution returns the execution with id, or ports.ErrNotFound.
func (r *Repository) GetExecution(ctx context.Context, id int64) (execution.Execution, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+executionColumns+" FROM execution WHERE id = ?", id)
	c, err := scanExecution(row)
	if errors.Is(err, sql.ErrNoRows) {
		return execution.Execution{}, ports.ErrNotFound
	}
	if err != nil {
		return execution.Execution{}, fmt.Errorf("mysql: get execution: %w", err)
	}
	return c, nil
}

// ListExecutionsByProject returns all executions belonging to projectID.
func (r *Repository) ListExecutionsByProject(ctx context.Context, projectID int64) ([]execution.Execution, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+executionColumns+" FROM execution WHERE project_id = ?", projectID)
	if err != nil {
		return nil, fmt.Errorf("mysql: list executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []execution.Execution{}
	for rows.Next() {
		c, scanErr := scanExecution(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan execution: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate executions: %w", err)
	}
	return out, nil
}

// DeleteExecution removes the execution with id, or ports.ErrNotFound.
func (r *Repository) DeleteExecution(ctx context.Context, id int64) error {
	return execDelete(ctx, r.db, "DELETE FROM execution WHERE id = ?", id)
}

// AddExecutionFile records a data file for the execution, or
// ports.ErrFileExists on duplicate.
func (r *Repository) AddExecutionFile(ctx context.Context, executionID int64, filename string) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO execution_data (execution_id, filename) VALUES (?, ?)", executionID, filename)
	if isDuplicateKey(err) {
		return ports.ErrFileExists
	}
	if err != nil {
		return fmt.Errorf("mysql: add execution file: %w", err)
	}
	return nil
}

// ExecutionFilesFor returns the execution's data files.
func (r *Repository) ExecutionFilesFor(ctx context.Context, executionID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT filename FROM execution_data WHERE execution_id = ?", executionID)
	if err != nil {
		return nil, fmt.Errorf("mysql: execution files: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return nil, fmt.Errorf("mysql: scan execution file: %w", scanErr)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate execution files: %w", err)
	}
	return out, nil
}

// DeleteExecutionFile removes a data file record, or ports.ErrNotFound.
func (r *Repository) DeleteExecutionFile(ctx context.Context, executionID int64, filename string) error {
	return execDelete(ctx, r.db, "DELETE FROM execution_data WHERE execution_id = ? AND filename = ?", executionID, filename)
}

// StoreLoadProfile replaces the execution's execution scenarios and updates
// its csv_split flag atomically. Returns ports.ErrNotFound if the execution
// does not exist.
func (r *Repository) StoreLoadProfile(ctx context.Context, executionID int64, csvSplit bool, scenarios []loadprofile.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mysql: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM execution WHERE id = ?)", executionID).Scan(&exists); err != nil {
		return fmt.Errorf("mysql: check execution: %w", err)
	}
	if !exists {
		return ports.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM execution_scenario WHERE execution_id = ?", executionID); err != nil {
		return fmt.Errorf("mysql: clear execution scenarios: %w", err)
	}
	for _, ep := range scenarios {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO execution_scenario (execution_id, scenario_id, concurrency, rampup, duration, engines, csv_split) VALUES (?, ?, ?, ?, ?, ?, ?)",
			executionID, ep.ScenarioID, ep.Concurrency, ep.Rampup, ep.Duration, ep.Engines, boolToInt(ep.CSVSplit),
		); err != nil {
			return fmt.Errorf("mysql: insert execution scenario: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE execution SET csv_split = ? WHERE id = ?", boolToInt(csvSplit), executionID); err != nil {
		return fmt.Errorf("mysql: update csv_split: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mysql: commit: %w", err)
	}
	return nil
}

// LoadProfileFor returns the execution's current execution scenarios. Scenario names
// are not persisted, so ExecutionScenario.Name is empty.
func (r *Repository) LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT scenario_id, concurrency, rampup, duration, engines, csv_split FROM execution_scenario WHERE execution_id = ?", executionID)
	if err != nil {
		return nil, fmt.Errorf("mysql: execution scenarios: %w", err)
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
			return nil, fmt.Errorf("mysql: scan execution scenario: %w", scanErr)
		}
		ep.Engines = int(engines.Int64)
		ep.CSVSplit = csvSplit != 0
		out = append(out, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate execution scenarios: %w", err)
	}
	return out, nil
}

func scanExecution(s rowScanner) (execution.Execution, error) {
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
