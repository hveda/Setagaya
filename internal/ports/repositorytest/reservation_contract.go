package repositorytest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewReservationRepo builds a fresh, empty ReservationRepository for one test.
type NewReservationRepo func(t *testing.T) ports.ReservationRepository

func at(seconds int) time.Time { return time.Unix(int64(seconds), 0).UTC() }

// RunReservationRepositoryContract pins the behaviour every
// ReservationRepository must share.
func RunReservationRepositoryContract(t *testing.T, newRepo NewReservationRepo) {
	t.Helper()

	t.Run("CreateAndDelete", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateReservation(ctx, reservation.Reservation{
			TenantID: 1, Cluster: "default", EngineCount: 3,
			Start: at(0), End: at(60), ExecutionID: 100,
		})
		if err != nil {
			t.Fatalf("CreateReservation: %v", err)
		}
		if id <= 0 {
			t.Fatalf("CreateReservation id = %d, want > 0", id)
		}

		got, err := repo.ReservationsInWindow(ctx, 1, "default", at(0), at(60))
		if err != nil {
			t.Fatalf("ReservationsInWindow: %v", err)
		}
		if len(got) != 1 || got[0].EngineCount != 3 || got[0].ExecutionID != 100 {
			t.Fatalf("ReservationsInWindow = %+v, want the one reservation just created", got)
		}

		if err := repo.DeleteReservation(ctx, id); err != nil {
			t.Fatalf("DeleteReservation: %v", err)
		}
		got, err = repo.ReservationsInWindow(ctx, 1, "default", at(0), at(60))
		if err != nil {
			t.Fatalf("ReservationsInWindow after delete: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("ReservationsInWindow after delete = %+v, want none -- deleting frees capacity immediately", got)
		}

		if err := repo.DeleteReservation(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteReservation (already gone) = %v, want ErrNotFound", err)
		}
	})

	// The exact case a naive query would get wrong: a reservation whose
	// window straddles the query window's boundary must still be found, and
	// one that merely touches it (ends exactly where the query starts, or
	// starts exactly where it ends) must not.
	t.Run("InWindowMatchesOnlyTrueOverlaps", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		straddlesStart := mustReserve(t, repo, 1, "default", at(0), at(20), 1) // ends inside [10,30)
		straddlesEnd := mustReserve(t, repo, 1, "default", at(20), at(40), 2)  // starts inside [10,30)
		fullyInside := mustReserve(t, repo, 1, "default", at(15), at(18), 3)   // entirely inside
		mustReserve(t, repo, 1, "default", at(-10), at(10), 4)                 // ends exactly at query start
		mustReserve(t, repo, 1, "default", at(30), at(50), 5)                  // starts exactly at query end
		mustReserve(t, repo, 1, "default", at(-20), at(-5), 6)                 // entirely before
		mustReserve(t, repo, 1, "default", at(60), at(80), 7)                  // entirely after

		got, err := repo.ReservationsInWindow(ctx, 1, "default", at(10), at(30))
		if err != nil {
			t.Fatalf("ReservationsInWindow: %v", err)
		}
		gotExec := make(map[int64]bool, len(got))
		for _, r := range got {
			gotExec[r.ExecutionID] = true
		}
		for _, want := range []int64{straddlesStart, straddlesEnd, fullyInside} {
			if !gotExec[want] {
				t.Errorf("ReservationsInWindow missing execution %d, which genuinely overlaps [10,30)", want)
			}
		}
		if len(got) != 3 {
			t.Errorf("ReservationsInWindow = %d reservations, want exactly 3 (abutting/outside windows must not match): %+v", len(got), got)
		}
	})

	t.Run("InWindowScopesByTenantAndCluster", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		mustReserve(t, repo, 1, "default", at(0), at(100), 1)
		mustReserve(t, repo, 2, "default", at(0), at(100), 2) // different tenant
		mustReserve(t, repo, 1, "eu-west", at(0), at(100), 3) // different cluster

		got, err := repo.ReservationsInWindow(ctx, 1, "default", at(0), at(100))
		if err != nil {
			t.Fatalf("ReservationsInWindow: %v", err)
		}
		if len(got) != 1 || got[0].ExecutionID != 1 {
			t.Fatalf("ReservationsInWindow = %+v, want only tenant 1's default-cluster reservation", got)
		}
	})
}

func mustReserve(t *testing.T, repo ports.ReservationRepository, tenantID int64, cluster string, start, end time.Time, executionID int64) int64 {
	t.Helper()
	_, err := repo.CreateReservation(context.Background(), reservation.Reservation{
		TenantID: tenantID, Cluster: cluster, EngineCount: 1,
		Start: start, End: end, ExecutionID: executionID,
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	return executionID
}
