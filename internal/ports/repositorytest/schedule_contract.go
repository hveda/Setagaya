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

	t.Run("ClaimDueOccurrenceReturnsFalseWhenNothingIsDue", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateSchedule(ctx, schedule.Schedule{ExecutionID: 1, TenantID: 7, Kind: schedule.KindRecurring, Recurrence: "* * * * *"})
		if err != nil {
			t.Fatalf("CreateSchedule: %v", err)
		}
		// Reserved but not due yet, and rejected (never claimable regardless
		// of fire time) -- neither should ever be returned.
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: id, FireTime: at(1000), Status: ports.OccurrenceReserved}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: id, FireTime: at(0), Status: ports.OccurrenceRejected}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}

		_, found, err := repo.ClaimDueOccurrence(ctx, at(500))
		if err != nil {
			t.Fatalf("ClaimDueOccurrence: %v", err)
		}
		if found {
			t.Fatalf("ClaimDueOccurrence found = true, want false -- nothing reserved is due yet")
		}
	})

	// The claim is the exclusive hand-off cmd/scheduler relies on: the
	// earliest due, still-reserved occurrence is returned and immediately
	// marked fired so a second call (or a second replica) never sees it
	// again, and occurrences due later are claimed in fire-time order.
	t.Run("ClaimDueOccurrenceClaimsEarliestDueFirstAndMarksItFired", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateSchedule(ctx, schedule.Schedule{ExecutionID: 1, TenantID: 7, Kind: schedule.KindRecurring, Recurrence: "* * * * *"})
		if err != nil {
			t.Fatalf("CreateSchedule: %v", err)
		}
		earlyReservation := int64(11)
		lateReservation := int64(22)
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{
			ScheduleID: id, FireTime: at(100), Status: ports.OccurrenceReserved, ReservationID: &lateReservation,
		}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{
			ScheduleID: id, FireTime: at(50), Status: ports.OccurrenceReserved, ReservationID: &earlyReservation,
		}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err)
		}
		if _, err := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: id, FireTime: at(1000), Status: ports.OccurrenceReserved}); err != nil {
			t.Fatalf("CreateOccurrence: %v", err) // due later; must not be claimed yet
		}

		now := at(200) // both t=50 and t=100 are due; t=1000 is not
		got, found, err := repo.ClaimDueOccurrence(ctx, now)
		if err != nil {
			t.Fatalf("ClaimDueOccurrence: %v", err)
		}
		if !found || !got.FireTime.Equal(at(50)) || got.ReservationID == nil || *got.ReservationID != earlyReservation {
			t.Fatalf("ClaimDueOccurrence first claim = %+v, want the t=50 occurrence (earliest due)", got)
		}
		if got.Status != ports.OccurrenceFired {
			t.Errorf("claimed occurrence status = %v, want fired", got.Status)
		}

		got, found, err = repo.ClaimDueOccurrence(ctx, now)
		if err != nil {
			t.Fatalf("ClaimDueOccurrence (second): %v", err)
		}
		if !found || !got.FireTime.Equal(at(100)) || got.ReservationID == nil || *got.ReservationID != lateReservation {
			t.Fatalf("ClaimDueOccurrence second claim = %+v, want the t=100 occurrence", got)
		}

		// Both due occurrences are now claimed; only the not-yet-due one is left.
		_, found, err = repo.ClaimDueOccurrence(ctx, now)
		if err != nil {
			t.Fatalf("ClaimDueOccurrence (third): %v", err)
		}
		if found {
			t.Fatalf("ClaimDueOccurrence found a third occurrence, want none left due")
		}

		occs, err := repo.OccurrencesForSchedule(ctx, id)
		if err != nil {
			t.Fatalf("OccurrencesForSchedule: %v", err)
		}
		fired := 0
		for _, o := range occs {
			if o.Status == ports.OccurrenceFired {
				fired++
			}
		}
		if fired != 2 {
			t.Fatalf("fired occurrences in storage = %d, want 2 (the claims must persist)", fired)
		}
	})

	// The horizon-extension pass only ever needs recurring, active schedules:
	// a one-shot has exactly one occurrence and nothing to extend, and an
	// inactive schedule is paused, not rolled forward.
	t.Run("ListActiveRecurringSchedulesFiltersKindAndActive", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wantID, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 1, TenantID: 7, Kind: schedule.KindRecurring, Recurrence: "* * * * *", Active: true,
		})
		if err != nil {
			t.Fatalf("CreateSchedule (active recurring): %v", err)
		}
		if _, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 2, TenantID: 7, Kind: schedule.KindRecurring, Recurrence: "* * * * *", Active: false,
		}); err != nil {
			t.Fatalf("CreateSchedule (inactive recurring): %v", err)
		}
		fireAt := at(100)
		if _, err := repo.CreateSchedule(ctx, schedule.Schedule{
			ExecutionID: 3, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
		}); err != nil {
			t.Fatalf("CreateSchedule (active one-shot): %v", err)
		}

		got, err := repo.ListActiveRecurringSchedules(ctx)
		if err != nil {
			t.Fatalf("ListActiveRecurringSchedules: %v", err)
		}
		if len(got) != 1 || got[0].ID != wantID {
			t.Fatalf("ListActiveRecurringSchedules = %+v, want only the active recurring schedule", got)
		}
	})

	t.Run("HorizonExtensionRoundTrips", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		if _, found, err := repo.LastHorizonExtension(ctx); err != nil || found {
			t.Fatalf("LastHorizonExtension before any run = found:%v, err:%v, want found:false", found, err)
		}

		if err := repo.RecordHorizonExtension(ctx, at(100)); err != nil {
			t.Fatalf("RecordHorizonExtension: %v", err)
		}
		got, found, err := repo.LastHorizonExtension(ctx)
		if err != nil {
			t.Fatalf("LastHorizonExtension: %v", err)
		}
		if !found || !got.Equal(at(100)) {
			t.Fatalf("LastHorizonExtension = %v, found:%v, want %v, found:true", got, found, at(100))
		}

		// A later run overwrites the earlier one -- only the most recent
		// success is ever queryable.
		if err := repo.RecordHorizonExtension(ctx, at(200)); err != nil {
			t.Fatalf("RecordHorizonExtension (second): %v", err)
		}
		got, found, err = repo.LastHorizonExtension(ctx)
		if err != nil {
			t.Fatalf("LastHorizonExtension (after second run): %v", err)
		}
		if !found || !got.Equal(at(200)) {
			t.Fatalf("LastHorizonExtension after second run = %v, found:%v, want %v, found:true", got, found, at(200))
		}
	})
}
