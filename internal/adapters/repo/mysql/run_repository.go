package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// StartRun opens the active run for a collection and its history row.
func (r *Repository) StartRun(ctx context.Context, executionID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO collection_run (collection_id) VALUES (?)", executionID)
	if err != nil {
		if isDuplicateKey(err) {
			return 0, ports.ErrRunActive
		}
		return 0, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := r.db.ExecContext(ctx,
		"INSERT INTO collection_run_history (run_id, collection_id) VALUES (?, ?)", runID, executionID); err != nil {
		return 0, err
	}
	return runID, nil
}

// CurrentRun returns the active run id for a collection.
func (r *Repository) CurrentRun(ctx context.Context, executionID int64) (int64, bool, error) {
	var runID int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM collection_run WHERE collection_id=?", executionID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return runID, true, nil
}

// StopRun clears the active run and stamps its history end time.
func (r *Repository) StopRun(ctx context.Context, executionID int64) error {
	runID, ok, err := r.CurrentRun(ctx, executionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		"UPDATE collection_run_history SET end_time=NOW() WHERE collection_id=? AND run_id=?", executionID, runID); err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "DELETE FROM collection_run WHERE collection_id=?", executionID)
	return err
}

// MarkScenarioRunning records a running plan; duplicates are ignored (idempotent).
func (r *Repository) MarkScenarioRunning(ctx context.Context, executionID, scenarioID int64) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO running_plan (collection_id, plan_id, context) VALUES (?, ?, ?)",
		executionID, scenarioID, r.deployContext)
	if isDuplicateKey(err) {
		return nil
	}
	return err
}

// ClearScenarioRunning removes a running plan marker (idempotent).
func (r *Repository) ClearScenarioRunning(ctx context.Context, executionID, scenarioID int64) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM running_plan WHERE collection_id=? AND plan_id=?", executionID, scenarioID)
	return err
}

// RunningScenarios lists every running plan in this deployment context.
func (r *Repository) RunningScenarios(ctx context.Context) ([]ports.RunningScenario, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT collection_id, plan_id, started_time FROM running_plan WHERE context=?", r.deployContext)
	if err != nil {
		return nil, err
	}
	return scanRunningPlans(rows)
}

// RunningScenariosByExecution lists running plans for one collection.
func (r *Repository) RunningScenariosByExecution(ctx context.Context, executionID int64) ([]ports.RunningScenario, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT collection_id, plan_id, started_time FROM running_plan WHERE collection_id=?", executionID)
	if err != nil {
		return nil, err
	}
	return scanRunningPlans(rows)
}

func scanRunningPlans(rows *sql.Rows) ([]ports.RunningScenario, error) {
	defer func() { _ = rows.Close() }()
	var out []ports.RunningScenario
	for rows.Next() {
		var rp ports.RunningScenario
		if err := rows.Scan(&rp.ExecutionID, &rp.ScenarioID, &rp.StartedTime); err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
