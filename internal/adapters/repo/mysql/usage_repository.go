package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/heridotlife/honryu/internal/ports"
)

// StartLaunch opens a launch (execution_launch guard + history row).
func (r *Repository) StartLaunch(ctx context.Context, executionID int64, owner string, engines, vu int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, "INSERT INTO execution_launch (execution_id) VALUES (?)", executionID)
	if err != nil {
		if isDuplicateKey(err) {
			return ports.ErrLaunchActive
		}
		return err
	}
	launchID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO execution_launch_history (execution_id, context, owner, engines_count, vu, launch_id) VALUES (?, ?, ?, ?, ?, ?)",
		executionID, r.deployContext, owner, engines, vu, launchID); err != nil {
		return err
	}
	return tx.Commit()
}

// FinishLaunch stamps end_time/vu on the open launch and clears the guard.
func (r *Repository) FinishLaunch(ctx context.Context, executionID int64, vu int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var launchID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM execution_launch WHERE execution_id=?", executionID).Scan(&launchID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // nothing open
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE execution_launch_history SET end_time=NOW(), vu=? WHERE launch_id=?", vu, launchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM execution_launch WHERE execution_id=?", executionID); err != nil {
		return err
	}
	return tx.Commit()
}

// LaunchHistory returns finished launches within [from, to].
func (r *Repository) LaunchHistory(ctx context.Context, from, to time.Time) ([]ports.LaunchRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT execution_id, context, owner, engines_count, vu, started_time, end_time
		 FROM execution_launch_history
		 WHERE end_time IS NOT NULL AND started_time >= ? AND end_time <= ?`, from, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []ports.LaunchRecord
	for rows.Next() {
		var (
			rec     ports.LaunchRecord
			engines sql.NullInt64
			vu      sql.NullInt64
			endTime sql.NullTime
		)
		if err := rows.Scan(&rec.ExecutionID, &rec.Context, &rec.Owner, &engines, &vu, &rec.StartedTime, &endTime); err != nil {
			return nil, err
		}
		rec.Engines = int(engines.Int64)
		rec.VU = int(vu.Int64)
		if endTime.Valid {
			t := endTime.Time
			rec.EndTime = &t
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
