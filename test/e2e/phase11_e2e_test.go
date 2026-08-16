//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

type phase11Env struct {
	client    *http.Client
	url       string
	repo      *mysqladapter.Repository
	sched     *fake.Scheduler
	lifecycle *lifecycleapp.Service
}

func setupPhase11(t *testing.T) *phase11Env {
	t.Helper()
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sink, bus, repo, repo)
	lifecycle := lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("jmeter")).WithMetrics(collector)

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:            projectapp.NewService(repo),
		Scenarios:           scenarioapp.NewService(repo, store),
		Executions:          executionapp.NewService(repo, store, 500),
		Lifecycle:           lifecycle,
		Store:               store,
		Metrics:             collector,
		Reports:             repo,
		IngestToken:         "engine-token",
		DefaultOwners:       []string{"honryu"},
		TriggerReadyPoll:    5 * time.Millisecond,
		TriggerReadyTimeout: 2 * time.Second,
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &phase11Env{client: srv.Client(), url: srv.URL, repo: repo, sched: sched, lifecycle: lifecycle}
}

// postRaw posts and returns the response without asserting, for the cases
// whose status/body the test itself inspects.
func postRaw(t *testing.T, e *phase11Env, path string) *http.Response {
	t.Helper()
	resp, err := e.client.Post(e.url+path, "", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// orphan builds one orphaned shard completion, as Ingest records it when a
// Final arrives with no open run.
func orphan(executionID, scenarioID int64, shard int, exitCode *int) ports.OrphanCompletion {
	return ports.OrphanCompletion{
		ExecutionID: executionID, ScenarioID: scenarioID, ShardIndex: shard,
		ExitCode: exitCode, FinishedAt: time.Unix(1000, 0).UTC(),
	}
}

// seed prepares a project + scenario + execution + config over HTTP; the
// scenario is portable (declarative) unless jmx is true, in which case a native
// JMX file is uploaded instead.
func seedPhase11(t *testing.T, e *phase11Env, name string, engine string, jmx bool) (projectID, scenarioID, executionID int64) {
	t.Helper()
	projectID = postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {name}, "owner": {"honryu"}})
	form := url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}}
	scenarioID = postForm(t, e.client, e.url+"/api/scenarios", form)
	if jmx {
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioID)+"/files", "s.jmx", "<jmx/>")
	} else {
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioID)+"/requests", "requests.yml",
			"default-address: https://httpbin.pve.heri.life\nrequests:\n  - url: /headers\n")
	}
	execForm := url.Values{"name": {"run"}, "project_id": {itoa(projectID)}}
	if engine != "" {
		execForm["engine"] = []string{engine}
	}
	executionID = postForm(t, e.client, e.url+"/api/executions", execForm)
	putMultipart(t, e.client, e.url+"/api/executions/"+itoa(executionID)+"/config", "config.yaml",
		minimalConfig(executionID, scenarioID, ""))
	return projectID, scenarioID, executionID
}

// Trigger right behind a deploy used to 409 until the caller's own retry won
// the race (task 121, live): the handler now owns the bounded wait, so a
// back-to-back deploy->trigger succeeds with no client-side retry even when
// the first readiness probe reports the pods not there yet.
func TestPhase11_TriggerWaitsForReadiness(t *testing.T) {
	e := setupPhase11(t)
	_, _, executionID := seedPhase11(t, e, "readiness", "", true)

	e.sched.NotReadyCalls = 1 // one startup beat: Deploy's pods "not yet" once

	base := e.url + "/api/executions/" + itoa(executionID)
	postAction(t, e.client, base+"/deploy", http.StatusOK)
	// Immediately, no sleep, no retry loop.
	postAction(t, e.client, base+"/trigger", http.StatusOK)

	_, running, err := e.repo.CurrentRun(context.Background(), executionID)
	if err != nil || !running {
		t.Fatalf("CurrentRun after immediate trigger: running=%v err=%v", running, err)
	}
	postAction(t, e.client, base+"/stop", http.StatusOK)
	postAction(t, e.client, base+"/purge", http.StatusOK)
}

// A portable scenario cannot run on a script-only engine: the deploy fails
// fast with a 400 naming k6, not a 500 or a 3am engine-side death.
func TestPhase11_PortableOnScriptOnlyEngineFailsFast(t *testing.T) {
	e := setupPhase11(t)
	_, _, executionID := seedPhase11(t, e, "portable-k6", "k6", false)

	rec := postRaw(t, e, "/api/executions/"+itoa(executionID)+"/deploy")
	if rec.StatusCode != http.StatusBadRequest {
		t.Fatalf("deploy portable-on-k6 = %d, want 400", rec.StatusCode)
	}
	var errBody struct{ Message string }
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode deploy error: %v", err)
	}
	if !strings.Contains(errBody.Message, "k6") {
		t.Fatalf("deploy error = %q, want it to name k6", errBody.Message)
	}
}

// The stranded-run guard: engines that already pushed orphaned Finals cannot
// be triggered into a corpse run -- a typed 409 says re-deploy -- and a fresh
// deploy clears the evidence and triggers normally.
func TestPhase11_TriggerRefusesFinishedEngines(t *testing.T) {
	e := setupPhase11(t)
	ctx := context.Background()
	_, scenarioID, executionID := seedPhase11(t, e, "finished", "", true)

	base := e.url + "/api/executions/" + itoa(executionID)
	postAction(t, e.client, base+"/deploy", http.StatusOK)

	// The engine finished while nobody triggered: its Final arrived orphaned
	// and was recorded (here seeded straight, as Ingest leaves it).
	code := 0
	if err := e.repo.RecordOrphanCompletion(ctx, orphan(executionID, scenarioID, 0, &code)); err != nil {
		t.Fatalf("RecordOrphanCompletion: %v", err)
	}

	resp := postRaw(t, e, "/api/executions/"+itoa(executionID)+"/trigger")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("trigger over finished engines = %d, want 409", resp.StatusCode)
	}
	var body struct{ Message string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode trigger error: %v", err)
	}
	if !strings.Contains(body.Message, "redeploy") {
		t.Fatalf("trigger error = %q, want the re-deploy instruction", body.Message)
	}

	// A fresh deploy clears the orphans; trigger proceeds.
	postAction(t, e.client, base+"/deploy", http.StatusOK)
	postAction(t, e.client, base+"/trigger", http.StatusOK)
	postAction(t, e.client, base+"/stop", http.StatusOK)
	postAction(t, e.client, base+"/purge", http.StatusOK)
}

// A run stranded open with full orphan coverage reconciles to an
// evidence-based report, exactly once, and the execution is operable again.
func TestPhase11_ReconcileClosesStrandedRun(t *testing.T) {
	e := setupPhase11(t)
	ctx := context.Background()
	_, scenarioID, executionID := seedPhase11(t, e, "stranded", "", true)

	base := e.url + "/api/executions/" + itoa(executionID)
	postAction(t, e.client, base+"/deploy", http.StatusOK)
	postAction(t, e.client, base+"/trigger", http.StatusOK)
	runID, running, err := e.repo.CurrentRun(ctx, executionID)
	if err != nil || !running {
		t.Fatalf("CurrentRun: running=%v err=%v", running, err)
	}

	// Every shard finished while the run stood open: exit 3 (criteria
	// failed) is stronger evidence than the abort baseline.
	code := 3
	if err := e.repo.RecordOrphanCompletion(ctx, orphan(executionID, scenarioID, 0, &code)); err != nil {
		t.Fatalf("RecordOrphanCompletion: %v", err)
	}

	// The service-level Reconcile is what the ticker drives in cmd/api.
	if err := e.lifecycle.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if err := e.lifecycle.Reconcile(ctx); err != nil { // idempotent
		t.Fatalf("Reconcile again: %v", err)
	}

	var rep struct {
		RunID   int64  `json:"run_id"`
		Outcome string `json:"outcome"`
	}
	getJSON(t, e.client, e.url+"/api/runs/"+itoa(runID)+"/report", http.StatusOK, &rep)
	if rep.RunID != runID || rep.Outcome != "failed" {
		t.Fatalf("reconciled report = %+v, want run %d failed (orphan exit 3)", rep, runID)
	}

	if _, running, _ := e.repo.CurrentRun(ctx, executionID); running {
		t.Fatal("reconcile left the stranded run open")
	}
	postAction(t, e.client, base+"/purge", http.StatusOK)
}
