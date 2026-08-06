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

func TestDelete_MissingSchedule(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := newService(store, time.Now())
	if err := svc.Delete(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}
}
