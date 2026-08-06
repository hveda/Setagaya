package scheduleapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// seedExecution creates a project and execution with a single-scenario load
// profile (engines, duration in seconds), and returns the execution's id.
// scheduleapp.Create only reads the load profile (LoadProfileFor), so no
// scenario file/upload plumbing is needed.
func seedExecution(t *testing.T, store *fake.Store, engines, durationSeconds int) int64 {
	t.Helper()
	ctx := context.Background()
	p, _ := project.New("web", "honryu", "")
	projectID, err := store.CreateProject(ctx, p)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	e, _ := execution.New("peak", projectID)
	executionID, err := store.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	entries := []loadprofile.Entry{
		{ScenarioID: 1, Concurrency: 10, Engines: engines, Duration: durationSeconds},
	}
	if err := store.StoreLoadProfile(ctx, executionID, false, entries); err != nil {
		t.Fatalf("StoreLoadProfile: %v", err)
	}
	return executionID
}

func newService(store *fake.Store, now time.Time) *scheduleapp.Service {
	return scheduleapp.NewService(store, quotaapp.NewService(store)).WithNow(func() time.Time { return now })
}

func TestCreate_OneShot_AdmitsAndReserves(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 2, 30) // 2 engines, 30s duration
	if err := store.SetCeiling(ctx, tenantID, "default", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fireAt := now.Add(time.Hour)
	svc := newService(store, now)

	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Cluster: "default",
		Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Schedule.ID <= 0 {
		t.Fatalf("Create schedule id = %d, want > 0", view.Schedule.ID)
	}
	if len(view.Occurrences) != 1 {
		t.Fatalf("Occurrences = %d, want 1", len(view.Occurrences))
	}
	occ := view.Occurrences[0]
	if occ.Status != ports.OccurrenceReserved || occ.ReservationID == nil {
		t.Fatalf("occ = %+v, want reserved with a reservation id", occ)
	}

	got, err := store.ReservationsInWindow(ctx, tenantID, "default", fireAt, fireAt.Add(time.Second))
	if err != nil {
		t.Fatalf("ReservationsInWindow: %v", err)
	}
	if len(got) != 1 || got[0].ID != *occ.ReservationID || got[0].EngineCount != 2 {
		t.Fatalf("ReservationsInWindow = %+v, want the occurrence's own 2-engine reservation", got)
	}
}

func TestCreate_OneShot_RejectsWhenOverQuota(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 5, 30) // 5 engines wanted
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fireAt := now.Add(time.Hour)
	svc := newService(store, now)

	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(view.Occurrences) != 1 {
		t.Fatalf("Occurrences = %d, want 1", len(view.Occurrences))
	}
	occ := view.Occurrences[0]
	if occ.Status != ports.OccurrenceRejected || occ.ReservationID != nil {
		t.Fatalf("occ = %+v, want rejected with no reservation id", occ)
	}

	got, err := store.ReservationsInWindow(ctx, tenantID, "", fireAt, fireAt.Add(time.Second))
	if err != nil {
		t.Fatalf("ReservationsInWindow: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ReservationsInWindow = %+v, want none -- a rejected occurrence must not hold capacity", got)
	}
}

// A one-shot schedule's fire time outside the 7-day admission horizon
// produces a schedule with no occurrences at all -- Create still succeeds
// (the schedule itself is a valid, storable request), it just has nothing
// reserved yet.
func TestCreate_OneShot_OutsideHorizonHasNoOccurrences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	executionID := seedExecution(t, store, 1, 30)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fireAt := now.AddDate(0, 0, 30) // 30 days out, well past the 7-day horizon
	svc := newService(store, now)

	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(view.Occurrences) != 0 {
		t.Fatalf("Occurrences = %+v, want none -- fire time is outside the admission horizon", view.Occurrences)
	}
}

func TestCreate_InvalidSchedule(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := newService(store, time.Now())
	_, err := svc.Create(context.Background(), schedule.Schedule{Kind: schedule.KindOneShot}) // no ExecutionID, no FireAt
	if !errors.Is(err, schedule.ErrExecutionRequired) {
		t.Fatalf("Create(invalid) = %v, want ErrExecutionRequired", err)
	}
}

func TestCreate_ExecutionWithNoLoadProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	p, _ := project.New("web", "honryu", "")
	projectID, _ := store.CreateProject(ctx, p)
	e, _ := execution.New("peak", projectID)
	executionID, _ := store.CreateExecution(ctx, e) // no load profile stored

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fireAt := now.Add(time.Hour)
	svc := newService(store, now)

	_, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true})
	if !errors.Is(err, loadprofile.ErrNoScenarios) {
		t.Fatalf("Create(no load profile) = %v, want ErrNoScenarios", err)
	}
}

// A recurring schedule whose occurrence windows overlap each other produces
// partial success driven purely by the ledger's real overlap accounting --
// not a contrived one-off rejection. Daily fire times 24h apart, each
// holding capacity for 30h, means every occurrence overlaps only the one
// right before it (30h < 48h, so it never reaches two occurrences back).
// With a ceiling that admits exactly one occurrence's engines at a time,
// admission alternates: reserved, rejected, reserved, rejected, ...
func TestCreate_Recurring_PartialSuccessFromOverlappingWindows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 2, 30*3600) // 2 engines, 30-hour duration
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // exactly a cron fire instant
	svc := newService(store, now)

	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(view.Occurrences) != 7 {
		t.Fatalf("Occurrences = %d, want 7 (one per day across the 7-day horizon)", len(view.Occurrences))
	}
	want := []ports.OccurrenceStatus{
		ports.OccurrenceReserved, ports.OccurrenceRejected, ports.OccurrenceReserved, ports.OccurrenceRejected,
		ports.OccurrenceReserved, ports.OccurrenceRejected, ports.OccurrenceReserved,
	}
	for i, occ := range view.Occurrences {
		if occ.Status != want[i] {
			t.Errorf("Occurrences[%d].Status = %v, want %v", i, occ.Status, want[i])
		}
		holdsReservation := occ.ReservationID != nil
		wantHolds := want[i] == ports.OccurrenceReserved
		if holdsReservation != wantHolds {
			t.Errorf("Occurrences[%d].ReservationID set = %v, want %v", i, holdsReservation, wantHolds)
		}
	}
}

func TestList_ReturnsEachSchedulesOccurrences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	executionID := seedExecution(t, store, 1, 30)
	other := seedExecution(t, store, 1, 30)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fireAt := now.Add(time.Hour)
	svc := newService(store, now)

	if _, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, schedule.Schedule{ExecutionID: other, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true}); err != nil {
		t.Fatalf("Create (other execution): %v", err)
	}

	views, err := svc.List(ctx, executionID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("List = %d schedules, want 1 (scoped to executionID)", len(views))
	}
	if len(views[0].Occurrences) != 1 {
		t.Fatalf("List[0].Occurrences = %d, want 1", len(views[0].Occurrences))
	}
}

func TestDelete_ReleasesReservedOccurrencesAndRemovesTheSchedule(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 2, 30)
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fireAt := now.Add(time.Hour)
	svc := newService(store, now)

	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	reservationID := *view.Occurrences[0].ReservationID

	if err := svc.Delete(ctx, view.Schedule.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := store.GetSchedule(ctx, view.Schedule.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("GetSchedule after delete = %v, want ErrNotFound", err)
	}
	if err := store.DeleteReservation(ctx, reservationID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("the occurrence's reservation was not released by Delete: DeleteReservation = %v, want ErrNotFound (already gone)", err)
	}
}

// Deleting a schedule must not touch a reservation belonging to something
// else for the same execution -- a manual trigger's reservation, or another
// schedule's -- even though both share the same executionID.
func TestDelete_DoesNotReleaseAnotherSchedulesReservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 1, 30)
	if err := store.SetCeiling(ctx, tenantID, "", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)

	fireAt1 := now.Add(time.Hour)
	view1, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt1, Active: true})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	fireAt2 := now.Add(2 * time.Hour)
	view2, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt2, Active: true})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	if err := svc.Delete(ctx, view1.Schedule.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	survivorID := *view2.Occurrences[0].ReservationID
	got, err := store.ReservationsInWindow(ctx, tenantID, "", fireAt2, fireAt2.Add(time.Second))
	if err != nil {
		t.Fatalf("ReservationsInWindow: %v", err)
	}
	if len(got) != 1 || got[0].ID != survivorID {
		t.Fatalf("ReservationsInWindow = %+v, want schedule 2's reservation still intact after deleting schedule 1", got)
	}
}

// A rejected occurrence never held a reservation -- Delete must skip it
// rather than trying (and failing) to release one, while still releasing
// every occurrence alongside it that did get admitted.
func TestDelete_SkipsRejectedOccurrencesButReleasesReservedOnes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 2, 30*3600) // 2 engines, 30-hour duration
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)

	// Same overlapping-window setup as the recurring partial-success test:
	// alternating reserved/rejected occurrences.
	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var reservationIDs []int64
	var sawRejected bool
	for _, occ := range view.Occurrences {
		switch occ.Status {
		case ports.OccurrenceReserved:
			reservationIDs = append(reservationIDs, *occ.ReservationID)
		case ports.OccurrenceRejected:
			sawRejected = true
		}
	}
	if len(reservationIDs) == 0 || !sawRejected {
		t.Fatalf("expected a mix of reserved and rejected occurrences, got %+v", view.Occurrences)
	}

	if err := svc.Delete(ctx, view.Schedule.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.GetSchedule(ctx, view.Schedule.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("GetSchedule after delete = %v, want ErrNotFound", err)
	}
	for _, id := range reservationIDs {
		if err := store.DeleteReservation(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("reservation %d was not released by Delete: DeleteReservation = %v, want ErrNotFound (already gone)", id, err)
		}
	}
}

func TestDelete_MissingSchedule(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := newService(store, time.Now())
	if err := svc.Delete(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}
}

func TestClaimDue_NothingDueReturnsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)
	executionID := seedExecution(t, store, 2, 30)
	if err := store.SetCeiling(ctx, 7, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	fireAt := now.Add(time.Hour) // not due yet
	if _, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, found, err := svc.ClaimDue(ctx, now)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if found {
		t.Fatalf("ClaimDue found = true, want false -- nothing is due yet")
	}
}

// Claiming a due occurrence releases the reservation it held: Trigger makes
// its own live one against the execution's current load profile, so holding
// onto the advance hold would double-count the same capacity.
func TestClaimDue_ClaimsDueOccurrenceAndReleasesItsReservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)
	executionID := seedExecution(t, store, 2, 30)
	if err := store.SetCeiling(ctx, tenantID, "eu-west", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	fireAt := now.Add(time.Minute)
	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Cluster: "eu-west", Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	reservationID := *view.Occurrences[0].ReservationID

	claim, found, err := svc.ClaimDue(ctx, fireAt) // exactly due
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if !found {
		t.Fatalf("ClaimDue found = false, want true -- the occurrence is due now")
	}
	if claim.Occurrence.ID != view.Occurrences[0].ID || claim.Schedule.ID != view.Schedule.ID {
		t.Fatalf("ClaimDue claim = %+v, want the occurrence/schedule just created", claim)
	}
	if claim.Schedule.ExecutionID != executionID || claim.Schedule.TenantID != tenantID || claim.Schedule.Cluster != "eu-west" {
		t.Fatalf("ClaimDue claim.Schedule = %+v, missing the fields cmd/scheduler needs to fire it", claim.Schedule)
	}

	if err := store.DeleteReservation(ctx, reservationID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("the occurrence's hold reservation was not released by ClaimDue: DeleteReservation = %v, want ErrNotFound (already gone)", err)
	}

	// Already claimed: a second call must not find it again.
	_, found, err = svc.ClaimDue(ctx, fireAt)
	if err != nil {
		t.Fatalf("ClaimDue (second): %v", err)
	}
	if found {
		t.Fatalf("ClaimDue found the same occurrence twice")
	}
}

// A rejected occurrence held no reservation to release -- ClaimDue must not
// error just because there was nothing to delete.
func TestClaimDue_RejectedOccurrenceNeverClaimable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)
	executionID := seedExecution(t, store, 5, 30) // wants more engines than the ceiling allows
	if err := store.SetCeiling(ctx, 7, "", 1); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	fireAt := now.Add(time.Minute)
	view, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Occurrences[0].Status != ports.OccurrenceRejected {
		t.Fatalf("occurrence status = %v, want rejected (over quota)", view.Occurrences[0].Status)
	}

	_, found, err := svc.ClaimDue(ctx, fireAt)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if found {
		t.Fatalf("ClaimDue found a rejected occurrence, want none claimable")
	}
}

func TestLastHorizonExtension_NeverRunReturnsFalse(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := newService(store, time.Now())
	if _, found, err := svc.LastHorizonExtension(context.Background()); err != nil || found {
		t.Fatalf("LastHorizonExtension before any run = found:%v, err:%v, want found:false", found, err)
	}
}

func TestExtendHorizons_NoActiveRecurringSchedulesStillRecordsCompletion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)

	if err := svc.ExtendHorizons(ctx); err != nil {
		t.Fatalf("ExtendHorizons: %v", err)
	}
	got, found, err := svc.LastHorizonExtension(ctx)
	if err != nil {
		t.Fatalf("LastHorizonExtension: %v", err)
	}
	if !found || !got.Equal(now) {
		t.Fatalf("LastHorizonExtension = %v, found:%v, want %v, found:true", got, found, now)
	}
}

// A recurring schedule's occurrences never re-appear as the horizon rolls
// forward: extension starts just after the latest already-known occurrence,
// and only reserves whatever new fire times fall within the new horizon.
func TestExtendHorizons_RollsRecurringScheduleForwardWithoutDuplicating(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 2, 30) // 2 engines, 30s duration
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, t0)
	view, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(view.Occurrences) != 7 {
		t.Fatalf("Occurrences after Create = %d, want 7 (Jan 1..7)", len(view.Occurrences))
	}

	// Three days later: the horizon (now+7d = Jan 11) reaches three new fire
	// times (Jan 8, 9, 10) past the ones Create already reserved (Jan 1..7).
	t1 := t0.AddDate(0, 0, 3)
	svc.WithNow(func() time.Time { return t1 })

	if err := svc.ExtendHorizons(ctx); err != nil {
		t.Fatalf("ExtendHorizons: %v", err)
	}

	occs, err := store.OccurrencesForSchedule(ctx, view.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule: %v", err)
	}
	if len(occs) != 10 {
		t.Fatalf("Occurrences after ExtendHorizons = %d, want 10 (7 original + 3 new)", len(occs))
	}
	wantLast := t0.AddDate(0, 0, 9) // Jan 10
	if last := occs[len(occs)-1].FireTime; !last.Equal(wantLast) {
		t.Fatalf("latest occurrence = %v, want %v", last, wantLast)
	}
	for _, occ := range occs {
		if occ.Status != ports.OccurrenceReserved {
			t.Errorf("occurrence at %v status = %v, want reserved (30s windows a day apart never overlap)", occ.FireTime, occ.Status)
		}
	}

	// Calling it again at the same "now" must not add anything further --
	// the horizon is already fully covered.
	if err := svc.ExtendHorizons(ctx); err != nil {
		t.Fatalf("ExtendHorizons (second, same now): %v", err)
	}
	occs2, err := store.OccurrencesForSchedule(ctx, view.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule: %v", err)
	}
	if len(occs2) != 10 {
		t.Fatalf("Occurrences after a second ExtendHorizons at the same now = %d, want still 10", len(occs2))
	}
}

// A one-shot schedule has exactly one occurrence and nothing to extend; an
// inactive recurring schedule is paused, not rolled forward. Neither should
// gain occurrences from ExtendHorizons.
func TestExtendHorizons_SkipsOneShotAndInactiveSchedules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)
	executionID := seedExecution(t, store, 1, 30)
	if err := store.SetCeiling(ctx, tenantID, "", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, now)

	fireAt := now.Add(time.Hour)
	oneShot, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindOneShot, FireAt: &fireAt, Active: true})
	if err != nil {
		t.Fatalf("Create (one-shot): %v", err)
	}
	inactive, err := svc.Create(ctx, schedule.Schedule{ExecutionID: executionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: false})
	if err != nil {
		t.Fatalf("Create (inactive recurring): %v", err)
	}

	svc.WithNow(func() time.Time { return now.AddDate(0, 0, 3) })
	if err := svc.ExtendHorizons(ctx); err != nil {
		t.Fatalf("ExtendHorizons: %v", err)
	}

	oneShotOccs, err := store.OccurrencesForSchedule(ctx, oneShot.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule (one-shot): %v", err)
	}
	if len(oneShotOccs) != 1 {
		t.Fatalf("one-shot occurrences after ExtendHorizons = %d, want still 1", len(oneShotOccs))
	}
	inactiveOccs, err := store.OccurrencesForSchedule(ctx, inactive.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule (inactive): %v", err)
	}
	if len(inactiveOccs) != len(inactive.Occurrences) {
		t.Fatalf("inactive schedule occurrences after ExtendHorizons = %d, want still %d", len(inactiveOccs), len(inactive.Occurrences))
	}
}

// A schedule that can no longer be extended (its execution's load profile
// was removed) must not stop the rest of the pass, and the pass must still
// record its own completion -- the whole point of a separate, always-updated
// timestamp is that a partial failure doesn't look like the job never ran.
func TestExtendHorizons_OneScheduleFailingDoesNotStopOthersOrSkipRecording(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	const tenantID = int64(7)

	brokenExecutionID := seedExecution(t, store, 2, 30)
	healthyExecutionID := seedExecution(t, store, 2, 30)
	if err := store.SetCeiling(ctx, tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc := newService(store, t0)
	broken, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: brokenExecutionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: true,
	})
	if err != nil {
		t.Fatalf("Create (broken): %v", err)
	}
	healthy, err := svc.Create(ctx, schedule.Schedule{
		ExecutionID: healthyExecutionID, TenantID: tenantID, Kind: schedule.KindRecurring, Recurrence: "0 0 * * *", Active: true,
	})
	if err != nil {
		t.Fatalf("Create (healthy): %v", err)
	}

	// The broken schedule's execution loses its load profile before the next
	// extension pass -- e.g. it was edited out from under an active schedule.
	if err := store.StoreLoadProfile(ctx, brokenExecutionID, false, nil); err != nil {
		t.Fatalf("StoreLoadProfile (clear): %v", err)
	}

	t1 := t0.AddDate(0, 0, 3)
	svc.WithNow(func() time.Time { return t1 })
	if err := svc.ExtendHorizons(ctx); err == nil {
		t.Fatal("ExtendHorizons: want an error surfacing the broken schedule, got nil")
	}

	healthyOccs, err := store.OccurrencesForSchedule(ctx, healthy.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule (healthy): %v", err)
	}
	if len(healthyOccs) != 10 {
		t.Fatalf("healthy schedule occurrences = %d, want 10 (7 original + 3 extended) -- the broken schedule must not have blocked it", len(healthyOccs))
	}
	brokenOccs, err := store.OccurrencesForSchedule(ctx, broken.Schedule.ID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule (broken): %v", err)
	}
	if len(brokenOccs) != 7 {
		t.Fatalf("broken schedule occurrences = %d, want still 7 (extension for it failed, nothing added)", len(brokenOccs))
	}

	got, found, err := svc.LastHorizonExtension(ctx)
	if err != nil {
		t.Fatalf("LastHorizonExtension: %v", err)
	}
	if !found || !got.Equal(t1) {
		t.Fatalf("LastHorizonExtension = %v, found:%v, want %v, found:true -- the pass must record completion even with a partial failure", got, found, t1)
	}
}
