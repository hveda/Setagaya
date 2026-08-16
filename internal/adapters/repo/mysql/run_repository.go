package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/heridotlife/honryu/internal/ports"
)

// StartRun opens the active run for a execution and its history row, stamping
// the deploy's correlation id onto it.
func (r *Repository) StartRun(ctx context.Context, executionID int64, correlationID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO execution_run (execution_id) VALUES (?)", executionID)
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
		"INSERT INTO execution_run_history (run_id, execution_id, correlation_id) VALUES (?, ?, ?)",
		runID, executionID, correlationID); err != nil {
		return 0, err
	}
	return runID, nil
}

// CurrentRun returns the active run id for a execution.
func (r *Repository) CurrentRun(ctx context.Context, executionID int64) (int64, bool, error) {
	var runID int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM execution_run WHERE execution_id=?", executionID).Scan(&runID)
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
		"UPDATE execution_run_history SET end_time=NOW() WHERE execution_id=? AND run_id=?", executionID, runID); err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "DELETE FROM execution_run WHERE execution_id=?", executionID)
	return err
}

// RunHistory returns a run's history record, or ports.ErrNotFound.
func (r *Repository) RunHistory(ctx context.Context, runID int64) (ports.RunRecord, error) {
	var rec ports.RunRecord
	var end sql.NullTime
	err := r.db.QueryRowContext(ctx,
		"SELECT run_id, execution_id, started_time, end_time, correlation_id FROM execution_run_history WHERE run_id=?",
		runID).Scan(&rec.RunID, &rec.ExecutionID, &rec.StartedTime, &end, &rec.CorrelationID)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.RunRecord{}, ports.ErrNotFound
	}
	if err != nil {
		return ports.RunRecord{}, err
	}
	if end.Valid {
		rec.EndTime = &end.Time
	}
	return rec, nil
}

// MarkScenarioRunning records a running scenario; duplicates are ignored (idempotent).
func (r *Repository) MarkScenarioRunning(ctx context.Context, executionID, scenarioID int64) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO running_scenario (execution_id, scenario_id, context) VALUES (?, ?, ?)",
		executionID, scenarioID, r.deployContext)
	if isDuplicateKey(err) {
		return nil
	}
	return err
}

// ClearScenarioRunning removes a running scenario marker (idempotent).
func (r *Repository) ClearScenarioRunning(ctx context.Context, executionID, scenarioID int64) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM running_scenario WHERE execution_id=? AND scenario_id=?", executionID, scenarioID)
	return err
}

// RunningScenarios lists every running scenario in this deployment context.
func (r *Repository) RunningScenarios(ctx context.Context) ([]ports.RunningScenario, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT execution_id, scenario_id, started_time FROM running_scenario WHERE context=?", r.deployContext)
	if err != nil {
		return nil, err
	}
	return scanRunningScenarios(rows)
}

// RunningScenariosByExecution lists running scenarios for one execution.
func (r *Repository) RunningScenariosByExecution(ctx context.Context, executionID int64) ([]ports.RunningScenario, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT execution_id, scenario_id, started_time FROM running_scenario WHERE execution_id=?", executionID)
	if err != nil {
		return nil, err
	}
	return scanRunningScenarios(rows)
}

func scanRunningScenarios(rows *sql.Rows) ([]ports.RunningScenario, error) {
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
