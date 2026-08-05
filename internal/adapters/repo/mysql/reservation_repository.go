package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/ports"
)

var _ ports.ReservationRepository = (*Repository)(nil)

// CreateReservation inserts r and returns its auto-assigned ID.
func (r *Repository) CreateReservation(ctx context.Context, res reservation.Reservation) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO reservation (tenant_id, cluster, engine_count, start_time, end_time, execution_id)"+
			" VALUES (?, ?, ?, ?, ?, ?)",
		res.TenantID, res.Cluster, res.EngineCount, res.Start, res.End, res.ExecutionID)
	if err != nil {
		return 0, fmt.Errorf("mysql: create reservation: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create reservation last id: %w", err)
	}
	return id, nil
}

// DeleteReservation removes the reservation with id, or ports.ErrNotFound.
func (r *Repository) DeleteReservation(ctx context.Context, id int64) error {
	return execDelete(ctx, r.db, "DELETE FROM reservation WHERE id = ?", id)
}

// ReservationsInWindow returns every reservation for tenant+cluster whose
// window overlaps [start, end): start_time < end AND end_time > start is the
// half-open-interval overlap test, matching reservation.Reservation.Overlaps.
func (r *Repository) ReservationsInWindow(ctx context.Context, tenantID int64, cluster string, start, end time.Time) ([]reservation.Reservation, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, tenant_id, cluster, engine_count, start_time, end_time, execution_id"+
			" FROM reservation WHERE tenant_id = ? AND cluster = ? AND start_time < ? AND end_time > ?",
		tenantID, cluster, end, start)
	if err != nil {
		return nil, fmt.Errorf("mysql: reservations in window: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []reservation.Reservation{}
	for rows.Next() {
		res, scanErr := scanReservation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan reservation: %w", scanErr)
		}
		out = append(out, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate reservations: %w", err)
	}
	return out, nil
}

func scanReservation(s rowScanner) (reservation.Reservation, error) {
	var res reservation.Reservation
	if err := s.Scan(&res.ID, &res.TenantID, &res.Cluster, &res.EngineCount, &res.Start, &res.End, &res.ExecutionID); err != nil {
		return reservation.Reservation{}, err
	}
	return res, nil
}
