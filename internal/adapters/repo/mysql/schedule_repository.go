package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/ports"
)

var _ ports.ScheduleRepository = (*Repository)(nil)

const scheduleColumns = "id, execution_id, tenant_id, cluster, kind, fire_at, recurrence, active"

// CreateSchedule inserts s and returns its auto-assigned ID.
func (r *Repository) CreateSchedule(ctx context.Context, s schedule.Schedule) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO schedule (execution_id, tenant_id, cluster, kind, fire_at, recurrence, active) VALUES (?, ?, ?, ?, ?, ?, ?)",
		s.ExecutionID, s.TenantID, s.Cluster, string(s.Kind), nullPtr(s.FireAt), nullString(s.Recurrence), s.Active)
	if err != nil {
		return 0, fmt.Errorf("mysql: create schedule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create schedule last id: %w", err)
	}
	return id, nil
}

// GetSchedule returns the schedule with id, or ports.ErrNotFound.
func (r *Repository) GetSchedule(ctx context.Context, id int64) (schedule.Schedule, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+scheduleColumns+" FROM schedule WHERE id = ?", id)
	s, err := scanSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, ports.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("mysql: get schedule: %w", err)
	}
	return s, nil
}

// ListSchedulesByExecution returns every schedule belonging to executionID.
func (r *Repository) ListSchedulesByExecution(ctx context.Context, executionID int64) ([]schedule.Schedule, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+scheduleColumns+" FROM schedule WHERE execution_id = ?", executionID)
	if err != nil {
		return nil, fmt.Errorf("mysql: list schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []schedule.Schedule{}
	for rows.Next() {
		s, scanErr := scanSchedule(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan schedule: %w", scanErr)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate schedules: %w", err)
	}
	return out, nil
}

// DeleteSchedule removes a schedule and every occurrence it owns, or
// ports.ErrNotFound. Both deletes commit together: a schedule gone with its
// occurrences left behind would orphan rows a later ScheduleID could collide
// with; occurrences gone with the schedule still present would leave a
// schedule silently empty.
func (r *Repository) DeleteSchedule(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mysql: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM schedule_occurrence WHERE schedule_id = ?", id); err != nil {
		return fmt.Errorf("mysql: delete schedule occurrences: %w", err)
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM schedule WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("mysql: delete schedule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mysql: delete schedule rows affected: %w", err)
	}
	if n == 0 {
		return ports.ErrNotFound
	}
	return tx.Commit()
}

func scanSchedule(s rowScanner) (schedule.Schedule, error) {
	var (
		sc         schedule.Schedule
		kind       string
		fireAt     sql.NullTime
		recurrence sql.NullString
	)
	if err := s.Scan(&sc.ID, &sc.ExecutionID, &sc.TenantID, &sc.Cluster, &kind, &fireAt, &recurrence, &sc.Active); err != nil {
		return schedule.Schedule{}, err
	}
	sc.Kind = schedule.Kind(kind)
	if fireAt.Valid {
		t := fireAt.Time
		sc.FireAt = &t
	}
	sc.Recurrence = recurrence.String
	return sc, nil
}

// CreateOccurrence inserts o and returns its auto-assigned ID.
func (r *Repository) CreateOccurrence(ctx context.Context, o ports.Occurrence) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO schedule_occurrence (schedule_id, fire_time, status, reservation_id) VALUES (?, ?, ?, ?)",
		o.ScheduleID, o.FireTime, string(o.Status), nullPtr(o.ReservationID))
	if err != nil {
		return 0, fmt.Errorf("mysql: create occurrence: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create occurrence last id: %w", err)
	}
	return id, nil
}

// OccurrencesForSchedule returns every occurrence belonging to scheduleID,
// ordered by fire time.
func (r *Repository) OccurrencesForSchedule(ctx context.Context, scheduleID int64) ([]ports.Occurrence, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, schedule_id, fire_time, status, reservation_id FROM schedule_occurrence WHERE schedule_id = ? ORDER BY fire_time",
		scheduleID)
	if err != nil {
		return nil, fmt.Errorf("mysql: occurrences for schedule: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ports.Occurrence{}
	for rows.Next() {
		var (
			o             ports.Occurrence
			status        string
			reservationID sql.NullInt64
		)
		if err := rows.Scan(&o.ID, &o.ScheduleID, &o.FireTime, &status, &reservationID); err != nil {
			return nil, fmt.Errorf("mysql: scan occurrence: %w", err)
		}
		o.Status = ports.OccurrenceStatus(status)
		if reservationID.Valid {
			id := reservationID.Int64
			o.ReservationID = &id
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate occurrences: %w", err)
	}
	return out, nil
}

// ClaimDueOccurrence locks and claims the earliest still-reserved occurrence
// due at or before now, in one short transaction (SELECT ... FOR UPDATE, then
// the UPDATE marking it fired, mirroring lockShard in report_progress.go) --
// two replicas racing this at once serialize on the lock, and only one sees
// the row before the other's transaction commits and moves it out of
// "reserved".
func (r *Repository) ClaimDueOccurrence(ctx context.Context, now time.Time) (ports.Occurrence, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ports.Occurrence{}, false, fmt.Errorf("mysql: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		o             ports.Occurrence
		reservationID sql.NullInt64
	)
	row := tx.QueryRowContext(ctx,
		"SELECT id, schedule_id, fire_time, reservation_id FROM schedule_occurrence"+
			" WHERE status = ? AND fire_time <= ? ORDER BY fire_time LIMIT 1 FOR UPDATE",
		string(ports.OccurrenceReserved), now)
	if err := row.Scan(&o.ID, &o.ScheduleID, &o.FireTime, &reservationID); errors.Is(err, sql.ErrNoRows) {
		return ports.Occurrence{}, false, nil
	} else if err != nil {
		return ports.Occurrence{}, false, fmt.Errorf("mysql: claim due occurrence: %w", err)
	}
	if reservationID.Valid {
		id := reservationID.Int64
		o.ReservationID = &id
	}
	o.Status = ports.OccurrenceFired

	if _, err := tx.ExecContext(ctx, "UPDATE schedule_occurrence SET status = ? WHERE id = ?", string(ports.OccurrenceFired), o.ID); err != nil {
		return ports.Occurrence{}, false, fmt.Errorf("mysql: mark occurrence fired: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ports.Occurrence{}, false, fmt.Errorf("mysql: commit claim: %w", err)
	}
	return o, true, nil
}
