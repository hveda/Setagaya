package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/heridotlife/honryu/internal/ports"
)

var _ ports.OrphanRepository = (*Repository)(nil)

// RecordOrphanCompletion stores an orphaned shard completion. A shard's Final
// is one event no matter how many times its sidecar retries the push, so the
// row is keyed by (execution, scenario, shard) with ON DUPLICATE KEY UPDATE
// rather than INSERT: a retry overwrites, it never accumulates.
func (r *Repository) RecordOrphanCompletion(ctx context.Context, oc ports.OrphanCompletion) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO execution_orphan_completion
			(execution_id, scenario_id, shard_index, exit_code, finished_at)
		VALUES (?,?,?,?,?)
		ON DUPLICATE KEY UPDATE exit_code = VALUES(exit_code), finished_at = VALUES(finished_at)`,
		oc.ExecutionID, oc.ScenarioID, oc.ShardIndex, oc.ExitCode, oc.FinishedAt.UTC())
	if err != nil {
		return fmt.Errorf("mysql: record orphan completion: %w", err)
	}
	return nil
}

// OrphanCompletions lists an execution's orphaned completions, oldest first --
// evidence in the order it arrived.
func (r *Repository) OrphanCompletions(ctx context.Context, executionID int64) ([]ports.OrphanCompletion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT execution_id, scenario_id, shard_index, exit_code, finished_at
		FROM execution_orphan_completion WHERE execution_id = ?
		ORDER BY finished_at, scenario_id, shard_index`, executionID)
	if err != nil {
		return nil, fmt.Errorf("mysql: orphan completions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ports.OrphanCompletion
	for rows.Next() {
		var (
			oc       ports.OrphanCompletion
			exitCode sql.NullInt64
		)
		if err := rows.Scan(&oc.ExecutionID, &oc.ScenarioID, &oc.ShardIndex, &exitCode, &oc.FinishedAt); err != nil {
			return nil, fmt.Errorf("mysql: scan orphan completion: %w", err)
		}
		if exitCode.Valid {
			code := int(exitCode.Int64)
			oc.ExitCode = &code
		}
		out = append(out, oc)
	}
	return out, rows.Err()
}

// ClearOrphanCompletions drops every orphan row for an execution.
func (r *Repository) ClearOrphanCompletions(ctx context.Context, executionID int64) error {
	if _, err := r.db.ExecContext(ctx,
		"DELETE FROM execution_orphan_completion WHERE execution_id = ?", executionID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("mysql: clear orphan completions: %w", err)
	}
	return nil
}
