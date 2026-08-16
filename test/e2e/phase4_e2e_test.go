//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/app/usageapp"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

// TestPhase4_DiagnosisEndToEnd drives a run that fails against a stub target
// the way Phase 0's spike did -- a mix of target-side 404s -- and asserts the
// spec AC this phase exists for: once the run is over and its engines purged,
// it is fully diagnosable from the API alone. Report, engine log, and the
// compiled config it actually ran are each retrieved by plain HTTP GET, never
// by reaching into the (fake) cluster.
func TestPhase4_DiagnosisEndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sink, bus, repo, repo)
	usage := usageapp.NewService(repo)
	lifecycle := lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("jmeter")).WithMetrics(collector).WithUsage(usage)
	admin := adminapp.NewService(repo, sched, lifecycle)

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Scenarios:     scenarioapp.NewService(repo, store),
		Executions:    executionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycle,
		Usage:         usage,
		Admin:         admin,
		Events:        bus,
		Store:         store,
		Metrics:       collector,
		Reports:       repo,
		IngestToken:   "engine-token",
		DefaultOwners: []string{"honryu"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	scenarioID := postForm(t, client, srv.URL+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})
	putMultipart(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/files", "scenario.jmx", "<jmx/>")
	executionID := postForm(t, client, srv.URL+"/api/executions", url.Values{"name": {"sale"}, "project_id": {itoa(projectID)}})
	cfg := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 10\n      rampup: 1\n      engines: 2\n      duration: 30\n",
		executionID, scenarioID)
	putMultipart(t, client, srv.URL+"/api/executions/"+itoa(executionID)+"/config", "config.yaml", cfg)

	base := srv.URL + "/api/executions/" + itoa(executionID)
	postAction(t, client, base+"/deploy", http.StatusOK)
	postAction(t, client, base+"/trigger", http.StatusOK)

	runID, running, err := repo.CurrentRun(context.Background(), executionID)
	if err != nil || !running {
		t.Fatalf("CurrentRun after trigger: %d, %v, %v", runID, running, err)
	}

	// Two shards, each pushing the wording apiritif actually produced against
	// the Phase 0 stub target for a 404, then reporting bzt's criteria-failed
	// exit code -- the failure mix and verdict mechanism Phase 0 confirmed.
	for shard := 0; shard < 2; shard++ {
		code := 3
		body, _ := json.Marshal(metrics.Batch{
			ExecutionID: executionID, ScenarioID: scenarioID, RunID: runID,
			ShardIndex: shard, StreamID: fmt.Sprintf("s%d", shard), Final: true, ExitCode: &code,
			Intervals: []metrics.Interval{{
				Seq: 1, Timestamp: 1000, Label: "checkout-cart",
				Concurrency: 5, Samples: 10, Succeeded: 7, Failed: 3,
				Latency: metrics.Histogram{0.01: 7, 0.3: 3},
				Errors: []metrics.ErrorGroup{
					{Message: "Request to http://target/cart didn't succeed (404)", ResponseCode: "404", Count: 3},
				},
			}},
		})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ingest", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer engine-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("ingest shard %d: %v", shard, err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("ingest shard %d = %d", shard, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	// The run finalised on its own -- no Stop was called -- because every
	// shard the profile called for went Final.
	var rep report.Report
	getJSON(t, client, srv.URL+"/api/runs/"+itoa(runID)+"/report", http.StatusOK, &rep)
	if rep.Outcome != taurus.OutcomeFailed {
		t.Fatalf("report outcome = %q, want failed", rep.Outcome)
	}
	if rep.Attribution.Target != 6 {
		t.Fatalf("target-attributed failures = %d, want 6 (3 per shard)", rep.Attribution.Target)
	}
	if len(rep.Errors) != 1 || rep.Errors[0].ResponseCode != "404" {
		t.Fatalf("errors = %+v, want one 404 signature", rep.Errors)
	}

	// The execution's report history lists it too.
	var history []report.Report
	getJSON(t, client, srv.URL+"/api/executions/"+itoa(executionID)+"/reports", http.StatusOK, &history)
	if len(history) != 1 || history[0].RunID != runID {
		t.Fatalf("execution report history = %+v", history)
	}

	// Purge captures each shard's log and the config it ran, then deletes the
	// (fake) pods -- after which nothing below reaches the scheduler again.
	postAction(t, client, base+"/purge", http.StatusOK)

	for shard := 0; shard < 2; shard++ {
		shardBase := fmt.Sprintf("%s/api/runs/%d/scenarios/%d/shards/%d", srv.URL, runID, scenarioID, shard)

		logResp, err := client.Get(shardBase + "/log")
		if err != nil {
			t.Fatalf("get log shard %d: %v", shard, err)
		}
		logBody := readAll(t, logResp)
		logResp.Body.Close()
		if logResp.StatusCode != http.StatusOK || len(logBody) == 0 {
			t.Fatalf("log shard %d = %d, %q", shard, logResp.StatusCode, logBody)
		}

		cfgResp, err := client.Get(shardBase + "/config")
		if err != nil {
			t.Fatalf("get config shard %d: %v", shard, err)
		}
		cfgBody := readAll(t, cfgResp)
		cfgResp.Body.Close()
		if cfgResp.StatusCode != http.StatusOK || len(cfgBody) == 0 {
			t.Fatalf("config shard %d = %d, %q", shard, cfgResp.StatusCode, cfgBody)
		}
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.String()
}
