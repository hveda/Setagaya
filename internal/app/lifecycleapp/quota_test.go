package lifecycleapp_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// setupWithTenant is setup's tenant-aware sibling: it seeds the same
// project/execution/scenario shape, but the execution declares tenantID (and
// the service is wired with a real quotaapp.Service) so quota gating in
// Trigger actually engages -- setup's executions all have a nil TenantID, so
// they never exercise this path.
func setupWithTenant(t *testing.T, tenantID int64, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()

	p, _ := project.New("web", "honryu", "")
	projectID, _ := store.CreateProject(ctx, p)
	coll, _ := execution.New("peak", projectID)
	coll.TenantID = &tenantID
	executionID, _ := store.CreateExecution(ctx, coll)

	obj := fake.NewObjectStore()
	var tests []loadprofile.Entry
	var planIDs []int64
	for _, n := range engines {
		pl, _ := scenario.NewNative("scenario", projectID, taurus.ExecutorJMeter)
		scenarioID, _ := store.CreateScenario(ctx, pl)
		if err := store.AddScenarioFile(ctx, scenarioID, "test.jmx", true); err != nil {
			t.Fatalf("add test file: %v", err)
		}
		if err := obj.Upload(ctx, fmt.Sprintf("scenario/%d/test.jmx", scenarioID), strings.NewReader("<jmx/>")); err != nil {
			t.Fatalf("upload test file: %v", err)
		}
		planIDs = append(planIDs, scenarioID)
		tests = append(tests, loadprofile.Entry{
			Name: "p", ScenarioID: scenarioID, Concurrency: 10, Rampup: 1, Engines: n, Duration: 30,
		})
	}
	if err := store.StoreLoadProfile(ctx, executionID, false, tests); err != nil {
		t.Fatalf("store load profile: %v", err)
	}

	sched := fake.NewScheduler()
	svc := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage(image)).WithQuota(quotaapp.NewService(store))
	return &env{store: store, sched: sched, obj: obj, svc: svc, projectID: projectID, executionID: executionID, planIDs: planIDs}
}

func TestTrigger_RejectsWhenOverQuota(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenant(t, tenantID, 2, 3) // 5 engines total
	ctx := context.Background()
	if err := e.store.SetCeiling(ctx, tenantID, "", 4); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if err := e.svc.Trigger(ctx, e.executionID); !errors.Is(err, quotaapp.ErrOverQuota) {
		t.Fatalf("Trigger over quota: err = %v, want ErrOverQuota", err)
	}
	if _, running, _ := e.store.CurrentRun(ctx, e.executionID); running {
		t.Error("a run started despite being rejected for quota -- no pod should have been marked running")
	}
}

func TestTrigger_AdmitsWhenUnderCeilingAndReservesEngines(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenant(t, tenantID, 2, 3) // 5 engines total
	ctx := context.Background()
	if err := e.store.SetCeiling(ctx, tenantID, "", 5); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	window := time.Now().Add(time.Hour)
	got, err := e.store.ReservationsInWindow(ctx, tenantID, "", time.Now(), window)
	if err != nil {
		t.Fatalf("ReservationsInWindow: %v", err)
	}
	if len(got) != 1 || got[0].EngineCount != 5 || got[0].ExecutionID != e.executionID {
		t.Fatalf("ReservationsInWindow = %+v, want one reservation of 5 engines for this execution", got)
	}
}

// Quota is opt-in with the tenant it scopes to: an execution that names none
// must trigger exactly as it always has, even with a ceiling configured that
// would reject everything if it were consulted.
func TestTrigger_SkipsQuotaWhenExecutionHasNoTenant(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2, 3) // setup's executions have a nil TenantID
	ctx := context.Background()
	if err := e.store.SetCeiling(ctx, 0, "", 0); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	e.svc = lifecycleapp.NewService(e.store, e.sched, e.obj, lifecycleapp.StaticImage(image)).WithQuota(quotaapp.NewService(e.store))

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger with no tenant must ignore quota entirely: %v", err)
	}
}

// setupWithTenantAndPodSize is setupWithTenant's pinned-pod-size sibling: the
// execution additionally declares CPU/Memory (as only a CalibrateEngine
// execution ever does, task 73), so Trigger's engine-equivalents scaling
// actually engages rather than falling back to one unit per declared engine.
func setupWithTenantAndPodSize(t *testing.T, tenantID int64, cpu, memory string, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()

	p, _ := project.New("web", "honryu", "")
	projectID, _ := store.CreateProject(ctx, p)
	coll, _ := execution.New("calibrate", projectID)
	coll.TenantID = &tenantID
	coll.CPU, coll.Memory = cpu, memory
	executionID, _ := store.CreateExecution(ctx, coll)

	obj := fake.NewObjectStore()
	var tests []loadprofile.Entry
	var planIDs []int64
	for _, n := range engines {
		pl, _ := scenario.NewNative("scenario", projectID, taurus.ExecutorJMeter)
		scenarioID, _ := store.CreateScenario(ctx, pl)
		if err := store.AddScenarioFile(ctx, scenarioID, "test.jmx", true); err != nil {
			t.Fatalf("add test file: %v", err)
		}
		if err := obj.Upload(ctx, fmt.Sprintf("scenario/%d/test.jmx", scenarioID), strings.NewReader("<jmx/>")); err != nil {
			t.Fatalf("upload test file: %v", err)
		}
		planIDs = append(planIDs, scenarioID)
		tests = append(tests, loadprofile.Entry{
			Name: "p", ScenarioID: scenarioID, Concurrency: 10, Rampup: 1, Engines: n, Duration: 30,
		})
	}
	if err := store.StoreLoadProfile(ctx, executionID, false, tests); err != nil {
		t.Fatalf("store load profile: %v", err)
	}

	sched := fake.NewScheduler()
	svc := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage(image)).WithQuota(quotaapp.NewService(store))
	return &env{store: store, sched: sched, obj: obj, svc: svc, projectID: projectID, executionID: executionID, planIDs: planIDs}
}

// reservedEngineCount deploys and triggers e's execution and returns the
// engine count its own quota reservation was made for.
func reservedEngineCount(t *testing.T, e *env, tenantID int64) int {
	t.Helper()
	ctx := context.Background()
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	got, err := e.store.ReservationsInWindow(ctx, tenantID, "", time.Now(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("ReservationsInWindow: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ReservationsInWindow = %+v, want exactly one reservation", got)
	}
	return got[0].EngineCount
}

// A pinned pod exactly at the baseline size (500m/512Mi) reserves no more
// than the ordinary "1 unit per declared engine" count -- the scaling only
// ever amplifies a reservation above baseline size, never below the
// declared engine count itself.
func TestTrigger_PinnedPodAtBaselineSizeReservesOneUnitPerEngine(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenantAndPodSize(t, tenantID, "500m", "512Mi", 2)
	if err := e.store.SetCeiling(context.Background(), tenantID, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if got := reservedEngineCount(t, e, tenantID); got != 2 {
		t.Fatalf("reserved = %d, want 2 (baseline-sized pod, no amplification)", got)
	}
}

// A pinned pod 4x baseline CPU reserves 4 engine-equivalents per declared
// engine, not a flat 1 -- the whole point of task 81: a single oversized
// calibration pod must occupy proportionally more of the tenant's ceiling.
func TestTrigger_PinnedPodLargerCPUReservesProportionallyMoreEngineUnits(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenantAndPodSize(t, tenantID, "2", "512Mi", 1) // 2 cores = 4x the 500m baseline
	if err := e.store.SetCeiling(context.Background(), tenantID, "", 4); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if got := reservedEngineCount(t, e, tenantID); got != 4 {
		t.Fatalf("reserved = %d, want 4 (2 cores / 500m baseline, ceil'd)", got)
	}
}

// The larger of CPU's and memory's own ratio to baseline governs -- here
// memory (4x) dominates CPU (1x).
func TestTrigger_PinnedPodLargerMemoryReservesProportionallyMoreEngineUnits(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenantAndPodSize(t, tenantID, "500m", "2Gi", 1) // 2Gi = 4x the 512Mi baseline
	if err := e.store.SetCeiling(context.Background(), tenantID, "", 4); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if got := reservedEngineCount(t, e, tenantID); got != 4 {
		t.Fatalf("reserved = %d, want 4 (2Gi / 512Mi baseline, ceil'd)", got)
	}
}

// A pinned pod smaller than baseline still occupies a whole scheduling slot
// -- the ratio floors at 1 unit per pod, never rounds down to 0.
func TestTrigger_PinnedPodSmallerThanBaselineStillReservesAtLeastOneUnitPerEngine(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenantAndPodSize(t, tenantID, "100m", "128Mi", 3)
	if err := e.store.SetCeiling(context.Background(), tenantID, "", 3); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if got := reservedEngineCount(t, e, tenantID); got != 3 {
		t.Fatalf("reserved = %d, want 3 (floored at 1 unit per engine, not 0)", got)
	}
}

// An over-quota tenant is rejected for a pinned oversized pod exactly as an
// ordinary Trigger is -- the amplified reservation is what makes it not fit,
// not a separate rejection path.
func TestTrigger_OverQuotaBecauseOfPinnedPodSizeAmplificationIsRejected(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenantAndPodSize(t, tenantID, "2", "512Mi", 1) // needs 4 units
	ctx := context.Background()
	if err := e.store.SetCeiling(ctx, tenantID, "", 3); err != nil { // a flat "1 engine" count would have fit; 4 does not
		t.Fatalf("SetCeiling: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); !errors.Is(err, quotaapp.ErrOverQuota) {
		t.Fatalf("Trigger over quota (via pod size amplification): err = %v, want ErrOverQuota", err)
	}
}

// A malformed pinned CPU/memory string fails loudly rather than silently
// mis-sizing the reservation.
func TestTrigger_MalformedPinnedPodSizePropagatesAnError(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	for _, tc := range []struct {
		name, cpu, memory string
	}{
		{"bad cpu", "not-a-quantity", "512Mi"},
		{"bad memory", "500m", "not-a-quantity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := setupWithTenantAndPodSize(t, tenantID, tc.cpu, tc.memory, 1)
			ctx := context.Background()
			if err := e.svc.Deploy(ctx, e.executionID); err != nil {
				t.Fatalf("Deploy: %v", err)
			}
			if err := e.svc.Trigger(ctx, e.executionID); err == nil {
				t.Fatal("Trigger with a malformed pinned pod size: expected error, got nil")
			}
		})
	}
}

func TestStop_ReleasesReservationImmediately(t *testing.T) {
	t.Parallel()
	const tenantID = int64(7)
	e := setupWithTenant(t, tenantID, 2)
	ctx := context.Background()
	if err := e.store.SetCeiling(ctx, tenantID, "", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	if err := e.svc.Stop(ctx, e.executionID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	got, err := e.store.ReservationsInWindow(ctx, tenantID, "", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("ReservationsInWindow: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ReservationsInWindow after Stop = %+v, want none -- Stop releases the reservation immediately, not at its declared end", got)
	}
}
