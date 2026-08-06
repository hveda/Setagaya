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
