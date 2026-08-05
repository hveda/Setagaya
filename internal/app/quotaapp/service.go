// Package quotaapp is the shared quota-check/reserve use-case: the one place
// that decides whether a new reservation fits a tenant's per-cluster engine
// ceiling. Both a manual Trigger (internal/app/lifecycleapp) and a scheduled
// occurrence's firing (internal/app/scheduleapp) call Reserve rather than
// each deciding admission its own way -- the guarantee a reservation is
// supposed to be would mean nothing if the two paths could disagree.
package quotaapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/ports"
)

// ErrOverQuota is returned when a new reservation would exceed the tenant's
// ceiling for its cluster.
var ErrOverQuota = errors.New("quotaapp: reservation would exceed quota")

// Service is the shared quota-check/reserve use-case.
type Service struct {
	repo ports.ReservationRepository
}

// NewService wires the quota service against its reservation ledger.
func NewService(repo ports.ReservationRepository) *Service {
	return &Service{repo: repo}
}

// Reserve admits a new reservation for tenantID+cluster if it fits the
// ceiling, creating and returning it. Admission sums every existing
// reservation whose window overlaps [start, end) -- not a running total --
// so the check is exact for the specific window being claimed, not an
// approximation across all time.
func (s *Service) Reserve(ctx context.Context, tenantID int64, cluster string, engineCount int, start, end time.Time, executionID int64) (reservation.Reservation, error) {
	r := reservation.Reservation{
		TenantID: tenantID, Cluster: cluster, EngineCount: engineCount,
		Start: start, End: end, ExecutionID: executionID,
	}
	if err := r.Validate(); err != nil {
		return reservation.Reservation{}, err
	}

	ceiling, err := s.repo.GetCeiling(ctx, tenantID, cluster)
	if err != nil {
		return reservation.Reservation{}, err
	}
	existing, err := s.repo.ReservationsInWindow(ctx, tenantID, cluster, start, end)
	if err != nil {
		return reservation.Reservation{}, err
	}
	used := 0
	for _, e := range existing {
		used += e.EngineCount
	}
	if used+engineCount > ceiling {
		return reservation.Reservation{}, fmt.Errorf(
			"%w: tenant %d cluster %q wants %d engines, %d already reserved in this window, ceiling %d",
			ErrOverQuota, tenantID, cluster, engineCount, used, ceiling)
	}

	id, err := s.repo.CreateReservation(ctx, r)
	if err != nil {
		return reservation.Reservation{}, err
	}
	r.ID = id
	return r, nil
}
