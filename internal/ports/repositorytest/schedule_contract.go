package repositorytest

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewScheduleRepo builds a fresh, empty ScheduleRepository for one test.
type NewScheduleRepo func(t *testing.T) ports.ScheduleRepository

// RunScheduleRepositoryContract pins the behaviour every ScheduleRepository
// must share.
func RunScheduleRepositoryContract(t *testing.T, newRepo NewScheduleRepo) {
	t.Helper()

	t.Run("CreateGetAndListByExecution", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		fireAt := at(100)

		id, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 1, TenantID: 7, Cluster: "default",
			Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
		})
		if err != nil {
			t.Fatalf("CreateSchedule: %v", err)
		}
		if id <= 0 {
			t.Fatalf("CreateSchedule id = %d, want > 0", id)
		}

		got, err := repo.GetSchedule(ctx, id)
		if err != nil {
			t.Fatalf("GetSchedule: %v", err)
		}
		if got.ExecutionID != 1 || got.TenantID != 7 || got.Cluster != "default" ||
			got.Kind != schedule.KindOneShot || got.FireAt == nil || !got.FireAt.Equal(fireAt) || !got.Active {
			t.Fatalf("GetSchedule = %+v, want the schedule just created", got)
		}

		// A second schedule on a different execution must not show up in the
		// first execution's list.
		if _, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 2, TenantID: 7, Cluster: "default", Kind: schedule.KindRecurring, Recurrence: "* * * * *",
		}); err != nil {
			t.Fatalf("CreateSchedule (other execution): %v", err)
		}
		list, err := repo.ListSchedulesByExecution(ctx, 1)
		if err != nil {
			t.Fatalf("ListSchedulesByExecution: %v", err)
		}
		if len(list) != 1 || list[0].ID != id {
			t.Fatalf("ListSchedulesByExecution = %+v, want only execution 1's schedule", list)
		}
	})

	t.Run("GetMissingReturnsNotFound", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.GetSchedule(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetSchedule(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("RecurringScheduleRoundTripsWithNoFireAt", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 1, TenantID: 7, Cluster: "eu-west",
			Kind: schedule.KindRecurring, Recurrence: "*/5 * * * *", Active: true,
		})
		if err != nil {
			t.Fatalf("CreateSchedule: %v", err)
		}
		got, err := repo.GetSchedule(ctx, id)
		if err != nil {
			t.Fatalf("GetSchedule: %v", err)
		}
		if got.Kind != schedule.KindRecurring || got.Recurrence != "*/5 * * * *" || got.FireAt != nil {
			t.Fatalf("GetSchedule = %+v, want a recurring schedule with no fire_at", got)
		}
	})

	// Deleting a schedule removes every occurrence it owns -- a schedule id
	// deleted and later reused (or simply a stray row) must never leave an
	// occurrence pointing at a schedule that no longer exists.
	t.Run("DeleteCascadesOccurrences", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 1, TenantID: 7, Cluster: "default", Kind: schedule.KindRecurring, Recurrence: "* * * * *",
		})
		if err != nil {
			t.Fatalf("CreateSchedule: %v", err)
		}
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: id, FireTime: at(0), Status: ports.OccurrenceReserved}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: id, FireTime: at(60), Status: ports.OccurrenceReserved}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}

		if err := repo.DeleteSchedule(ctx, id); err != nil {
			t.Fatalf("DeleteSchedule: %v", err)
		}
		if _, err := repo.GetSchedule(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetSchedule after delete = %v, want ErrNotFound", err)
		}
		occs, err := repo.OccurrencesForSchedule(ctx, id)
		if err != nil {
			t.Fatalf("OccurrencesForSchedule after delete: %v", err)
		}
		if len(occs) != 0 {
			t.Fatalf("OccurrencesForSchedule after delete = %+v, want none -- delete cascades", occs)
		}

		if err := repo.DeleteSchedule(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteSchedule (already gone) = %v, want ErrNotFound", err)
		}
	})

	// Every field on an occurrence round-trips, including the nullable
	// reservation id -- both while it holds one (reserved) and while it does
	// not (rejected, which never got a reservation at all).
	t.Run("OccurrencesRoundTripInFireTimeOrder", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 1, TenantID: 7, Cluster: "default", Kind: schedule.KindRecurring, Recurrence: "* * * * *",
		})
		if err != nil {
			t.Fatalf("CreateSchedule: %v", err)
		}

		reservationID := int64(42)
		// Created out of fire-time order, to prove OccurrencesForSchedule sorts
		// rather than merely returning insertion order.
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{
			ScheduleID: id, FireTime: at(120), Status: ports.OccurrenceRejected,
		}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{
			ScheduleID: id, FireTime: at(60), Status: ports.OccurrenceReserved, ReservationID: &reservationID,
		}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}

		occs, err := repo.OccurrencesForSchedule(ctx, id)
		if err != nil {
			t.Fatalf("OccurrencesForSchedule: %v", err)
		}
		if len(occs) != 2 {
			t.Fatalf("OccurrencesForSchedule = %d occurrences, want 2", len(occs))
		}
		if !occs[0].FireTime.Equal(at(60)) || occs[0].Status != ports.OccurrenceReserved ||
			occs[0].ReservationID == nil || *occs[0].ReservationID != reservationID {
			t.Errorf("occs[0] = %+v, want the reserved occurrence at t=60 with reservation id %d", occs[0], reservationID)
		}
		if !occs[1].FireTime.Equal(at(120)) || occs[1].Status != ports.OccurrenceRejected || occs[1].ReservationID != nil {
			t.Errorf("occs[1] = %+v, want the rejected occurrence at t=120 with no reservation id", occs[1])
		}
	})
}
