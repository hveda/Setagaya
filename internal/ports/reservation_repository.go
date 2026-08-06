package ports

import (
	"context"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
)

// ReservationRepository persists time-bounded engine-capacity reservations --
// the ledger that makes quota a guarantee rather than a best-effort check.
type ReservationRepository interface {
	// CreateReservation persists r and returns its assigned ID.
	CreateReservation(ctx context.Context, r reservation.Reservation) (int64, error)
	// DeleteReservation removes a reservation, freeing its capacity
	// immediately rather than waiting for its declared end time.
	DeleteReservation(ctx context.Context, id int64) error
	// ReservationsInWindow returns every reservation for tenant+cluster whose
	// window overlaps [start, end) -- the query a quota check runs to decide
	// whether a new reservation fits.
	ReservationsInWindow(ctx context.Context, tenantID int64, cluster string, start, end time.Time) ([]reservation.Reservation, error)

	// GetCeiling returns a tenant's engine quota ceiling for cluster, or 0 if
	// never configured -- nothing runs until a ceiling is explicitly set,
	// rather than defaulting to unlimited.
	GetCeiling(ctx context.Context, tenantID int64, cluster string) (int, error)
	// SetCeiling sets a tenant's per-cluster engine quota ceiling.
	SetCeiling(ctx context.Context, tenantID int64, cluster string, ceiling int) error

	// ReleaseReservationsForExecution deletes every reservation belonging to
	// executionID, freeing their capacity immediately. Not an error when
	// there are none -- an execution with no reservation (quota not
	// applicable, or already released) has nothing to release, which is a
	// normal outcome, not a fault.
	ReleaseReservationsForExecution(ctx context.Context, executionID int64) error
}
