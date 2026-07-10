package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// StartRun opens the active run for a collection and its history row.
func (r *Repository) StartRun(ctx context.Context, collectionID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO collection_run (collection_id) VALUES (?)", collectionID)
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
		"INSERT INTO collection_run_history (run_id, collection_id) VALUES (?, ?)", runID, collectionID); err != nil {
		return 0, err
	}
	return runID, nil
}

// CurrentRun returns the active run id for a collection.
func (r *Repository) CurrentRun(ctx context.Context, collectionID int64) (int64, bool, error) {
	var runID int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM collection_run WHERE collection_id=?", collectionID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return runID, true, nil
}

// StopRun clears the active run and stamps its history end time.
func (r *Repository) StopRun(ctx context.Context, collectionID int64) error {
	runID, ok, err := r.CurrentRun(ctx, collectionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		"UPDATE collection_run_history SET end_time=NOW() WHERE collection_id=? AND run_id=?", collectionID, runID); err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "DELETE FROM collection_run WHERE collection_id=?", collectionID)
	return err
}

// MarkPlanRunning records a running plan; duplicates are ignored (idempotent).
func (r *Repository) MarkPlanRunning(ctx context.Context, collectionID, planID int64) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO running_plan (collection_id, plan_id, context) VALUES (?, ?, ?)",
		collectionID, planID, r.deployContext)
	if isDuplicateKey(err) {
		return nil
	}
	return err
}

// ClearPlanRunning removes a running plan marker (idempotent).
func (r *Repository) ClearPlanRunning(ctx context.Context, collectionID, planID int64) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM running_plan WHERE collection_id=? AND plan_id=?", collectionID, planID)
	return err
}

// RunningPlans lists every running plan in this deployment context.
func (r *Repository) RunningPlans(ctx context.Context) ([]ports.RunningPlan, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT collection_id, plan_id, started_time FROM running_plan WHERE context=?", r.deployContext)
	if err != nil {
		return nil, err
	}
	return scanRunningPlans(rows)
}

// RunningPlansByCollection lists running plans for one collection.
func (r *Repository) RunningPlansByCollection(ctx context.Context, collectionID int64) ([]ports.RunningPlan, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT collection_id, plan_id, started_time FROM running_plan WHERE collection_id=?", collectionID)
	if err != nil {
		return nil, err
	}
	return scanRunningPlans(rows)
}

func scanRunningPlans(rows *sql.Rows) ([]ports.RunningPlan, error) {
	defer func() { _ = rows.Close() }()
	var out []ports.RunningPlan
	for rows.Next() {
		var rp ports.RunningPlan
		if err := rows.Scan(&rp.CollectionID, &rp.PlanID, &rp.StartedTime); err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
