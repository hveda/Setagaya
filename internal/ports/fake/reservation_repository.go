package fake

import (
	"context"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/ports"
)

// CreateReservation persists r and returns its assigned ID.
func (s *Store) CreateReservation(_ context.Context, r reservation.Reservation) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reservationSeq++
	r.ID = s.reservationSeq
	s.reservations[r.ID] = r
	return r.ID, nil
}

// DeleteReservation removes the reservation with id, or ports.ErrNotFound.
func (s *Store) DeleteReservation(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.reservations[id]; !ok {
		return ports.ErrNotFound
	}
	delete(s.reservations, id)
	return nil
}

// ReservationsInWindow returns every reservation for tenant+cluster whose
// window overlaps [start, end).
func (s *Store) ReservationsInWindow(_ context.Context, tenantID int64, cluster string, start, end time.Time) ([]reservation.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	probe := reservation.Reservation{Start: start, End: end}
	out := []reservation.Reservation{}
	for _, r := range s.reservations {
		if r.TenantID != tenantID || r.Cluster != cluster {
			continue
		}
		if probe.Overlaps(r) {
			out = append(out, r)
		}
	}
	return out, nil
}
