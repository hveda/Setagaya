package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/run"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

type lifecycleEnv struct {
	h           http.Handler
	store       *fake.Store
	sched       *fake.Scheduler
	exec        *fake.Executor
	executionID int64
	scenarioID  int64
	owner       string
}

// newLifecycleEnv wires a router with the lifecycle service and seeds an owned
// collection with one plan (JMX test file) and a stored execution config.
func newLifecycleEnv(t *testing.T, owner string) lifecycleEnv {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	sched := fake.NewScheduler()
	exec := fake.NewExecutor()

	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Plans:         scenarioapp.NewService(store, obj),
		Collections:   executionapp.NewService(store, obj, 100),
		Lifecycle:     lifecycleapp.NewService(store, sched, exec, obj, "img"),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})

	p, _ := project.New("web", owner, "")
	projectID, _ := store.CreateProject(ctx, p)
	coll, _ := execution.New("peak", projectID)
	executionID, _ := store.CreateExecution(ctx, coll)
	pl, _ := scenario.New("smoke", projectID)
	scenarioID, _ := store.CreateScenario(ctx, pl)
	if err := store.AddScenarioFile(ctx, scenarioID, "test.jmx", true); err != nil {
		t.Fatalf("add test file: %v", err)
	}
	if err := store.StoreLoadProfile(ctx, executionID, false, []loadprofile.Entry{
		{Name: "p", ScenarioID: scenarioID, Concurrency: 5, Rampup: 1, Engines: 2, Duration: 10},
	}); err != nil {
		t.Fatalf("store exec: %v", err)
	}
	return lifecycleEnv{h: h, store: store, sched: sched, exec: exec, executionID: executionID, scenarioID: scenarioID, owner: owner}
}

func TestLifecycleHTTP_DeployTriggerStatusStopPurge(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	base := "/api/executions/" + itoa(e.executionID)

	if rec := do(t, e.h, http.MethodPost, base+"/deploy"); rec.Code != http.StatusOK {
		t.Fatalf("deploy = %d (%s)", rec.Code, rec.Body.String())
	}

	// Status shows deployed engines.
	rec := do(t, e.h, http.MethodGet, base+"/status")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var st lifecycleapp.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if st.Phase != run.PhaseDeployed || st.PoolSize != 2 {
		t.Fatalf("status = %+v, want deployed/2", st)
	}

	if rec := do(t, e.h, http.MethodPost, base+"/trigger"); rec.Code != http.StatusOK {
		t.Fatalf("trigger = %d (%s)", rec.Code, rec.Body.String())
	}
	if e.exec.TriggerCount() != 2 {
		t.Fatalf("triggered engines = %d, want 2", e.exec.TriggerCount())
	}

	// Engines detail and pod log.
	if rec := do(t, e.h, http.MethodGet, base+"/engines"); rec.Code != http.StatusOK {
		t.Fatalf("engines = %d", rec.Code)
	}
	if rec := do(t, e.h, http.MethodGet, base+"/scenarios/"+itoa(e.scenarioID)+"/logs"); rec.Code != http.StatusOK {
		t.Fatalf("logs = %d", rec.Code)
	}

	if rec := do(t, e.h, http.MethodPost, base+"/stop"); rec.Code != http.StatusOK {
		t.Fatalf("stop = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := do(t, e.h, http.MethodPost, base+"/purge"); rec.Code != http.StatusOK {
		t.Fatalf("purge = %d", rec.Code)
	}
}

func TestLifecycleHTTP_TriggerBeforeDeployConflicts(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	base := "/api/executions/" + itoa(e.executionID)
	if rec := do(t, e.h, http.MethodPost, base+"/trigger"); rec.Code != http.StatusConflict {
		t.Fatalf("trigger before deploy = %d, want 409", rec.Code)
	}
}

func TestLifecycleHTTP_StopWithoutRunConflicts(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	base := "/api/executions/" + itoa(e.executionID)
	if rec := do(t, e.h, http.MethodPost, base+"/stop"); rec.Code != http.StatusConflict {
		t.Fatalf("stop without run = %d, want 409", rec.Code)
	}
}

func TestLifecycleHTTP_Forbidden(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "other-team")
	base := "/api/executions/" + itoa(e.executionID)
	for _, op := range []string{"/deploy", "/trigger", "/stop", "/purge"} {
		if rec := do(t, e.h, http.MethodPost, base+op); rec.Code != http.StatusForbidden {
			t.Errorf("%s on foreign collection = %d, want 403", op, rec.Code)
		}
	}
}

func TestLifecycleHTTP_InvalidIDs(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/executions/x/deploy"},
		{http.MethodPost, "/api/executions/x/trigger"},
		{http.MethodPost, "/api/executions/x/stop"},
		{http.MethodPost, "/api/executions/x/purge"},
		{http.MethodGet, "/api/executions/x/status"},
		{http.MethodGet, "/api/executions/x/engines"},
		{http.MethodGet, "/api/executions/x/scenarios/1/logs"},
		{http.MethodGet, "/api/executions/1/scenarios/x/logs"},
	}
	for _, tc := range cases {
		if rec := do(t, e.h, tc.method, tc.path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tc.method, tc.path, rec.Code)
		}
	}
}

func TestLifecycleHTTP_DeployMissingCollection(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	if rec := do(t, e.h, http.MethodPost, "/api/executions/9999/deploy"); rec.Code != http.StatusNotFound {
		t.Fatalf("deploy missing = %d, want 404", rec.Code)
	}
}

func TestLifecycleHTTP_DeployNoPlansIsBadRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newLifecycleEnv(t, "honryu")
	// A fresh owned collection with no execution config.
	c, _ := e.store.GetExecution(ctx, e.executionID)
	bare, _ := execution.New("bare", c.ProjectID)
	bareID, _ := e.store.CreateExecution(ctx, bare)

	rec := do(t, e.h, http.MethodPost, "/api/executions/"+itoa(bareID)+"/deploy")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("deploy no plans = %d, want 400", rec.Code)
	}
}

func TestLifecycleHTTP_EnginesMissingCollection(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	if rec := do(t, e.h, http.MethodGet, "/api/executions/9999/engines"); rec.Code != http.StatusNotFound {
		t.Fatalf("engines missing = %d, want 404", rec.Code)
	}
}

func TestLifecycleHTTP_PodLogNotDeployed(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t, "honryu")
	// No deploy: the plan's engines are unreachable -> 409 conflict.
	rec := do(t, e.h, http.MethodGet, "/api/executions/"+itoa(e.executionID)+"/scenarios/"+itoa(e.scenarioID)+"/logs")
	if rec.Code != http.StatusConflict {
		t.Fatalf("logs not deployed = %d, want 409", rec.Code)
	}
}
