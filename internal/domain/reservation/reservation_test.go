package reservation_test

import (
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
)

func at(seconds int) time.Time {
	return time.Unix(int64(seconds), 0).UTC()
}

func TestReservation_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		r    reservation.Reservation
		want error
	}{
		{"valid", reservation.Reservation{EngineCount: 1, Start: at(0), End: at(1)}, nil},
		{"zero engines", reservation.Reservation{EngineCount: 0, Start: at(0), End: at(1)}, reservation.ErrEngineCountInvalid},
		{"negative engines", reservation.Reservation{EngineCount: -1, Start: at(0), End: at(1)}, reservation.ErrEngineCountInvalid},
		{"end equals start", reservation.Reservation{EngineCount: 1, Start: at(1), End: at(1)}, reservation.ErrWindowInvalid},
		{"end before start", reservation.Reservation{EngineCount: 1, Start: at(1), End: at(0)}, reservation.ErrWindowInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.r.Validate()
			if tt.want == nil {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

// Overlaps is half-open: a reservation ending exactly when another starts
// does not collide with it, since the outgoing run's capacity is free from
// that instant. This is the boundary a naive <= / >= comparison gets wrong in
// either direction -- too loose (rejects legitimate back-to-back bookings) or
// too strict (admits real collisions one nanosecond inside the boundary).
func TestReservation_Overlaps(t *testing.T) {
	t.Parallel()
	base := reservation.Reservation{Start: at(10), End: at(20)}
	tests := []struct {
		name  string
		other reservation.Reservation
		want  bool
	}{
		{"identical window", reservation.Reservation{Start: at(10), End: at(20)}, true},
		{"fully contained", reservation.Reservation{Start: at(12), End: at(18)}, true},
		{"fully contains", reservation.Reservation{Start: at(5), End: at(25)}, true},
		{"overlaps the start", reservation.Reservation{Start: at(5), End: at(15)}, true},
		{"overlaps the end", reservation.Reservation{Start: at(15), End: at(25)}, true},
		{"abuts at the end, does not overlap", reservation.Reservation{Start: at(20), End: at(30)}, false},
		{"abuts at the start, does not overlap", reservation.Reservation{Start: at(0), End: at(10)}, false},
		{"one instant inside the end boundary overlaps", reservation.Reservation{Start: at(19), End: at(21)}, true},
		{"one instant inside the start boundary overlaps", reservation.Reservation{Start: at(9), End: at(11)}, true},
		{"entirely before", reservation.Reservation{Start: at(0), End: at(5)}, false},
		{"entirely after", reservation.Reservation{Start: at(25), End: at(30)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := base.Overlaps(tt.other); got != tt.want {
				t.Errorf("base.Overlaps(other) = %v, want %v", got, tt.want)
			}
			// Overlaps must be symmetric: whichever side asks, the answer is
			// the same collision, not a directional one.
			if got := tt.other.Overlaps(base); got != tt.want {
				t.Errorf("other.Overlaps(base) = %v, want %v (not symmetric)", got, tt.want)
			}
		})
	}
}

// Sub-nanosecond-scale adjacency, exercised with real time.Time arithmetic
// rather than the whole-second fixtures above, since that is where an
// off-by-one in the comparison operators would actually surface.
func TestReservation_OverlapsAtNanosecondBoundary(t *testing.T) {
	t.Parallel()
	start := time.Unix(1000, 0).UTC()
	end := start.Add(10 * time.Second)
	r := reservation.Reservation{Start: start, End: end}

	touching := reservation.Reservation{Start: end, End: end.Add(time.Second)}
	if r.Overlaps(touching) {
		t.Error("a reservation starting exactly at r's end must not overlap")
	}

	oneNanosecondEarly := reservation.Reservation{Start: end.Add(-time.Nanosecond), End: end.Add(time.Second)}
	if !r.Overlaps(oneNanosecondEarly) {
		t.Error("a reservation starting one nanosecond before r's end must overlap")
	}
}
