//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

// TestPhase19EndToEnd drives the journey phase 19 built (spec
// .cortex/2026-09-01-phase19-operator-ui) over real HTTP with a real MySQL
// container, closing review closure F5
// (.cortex/2026-09-02-phase19-review-closure): every prior phase has an e2e
// test, phase 19 had none.
//
// Covers, in the order an operator hits them: G5/G6/F4 (validate an
// uncompiled key, assert the info diagnostic names the key with neither a Go
// type nor yaml.v3's raw error shape leaking); G2/G3 (a raw text/yaml
// requests fragment stored and read back byte-exact); G7 (the JSON load
// profile, not multipart); deploy and trigger; and -- the part a unit test
// cannot prove -- the G8 ship gate for real: reading the run's ACTUAL
// compiled shard config back through GET .../shards/{shard}/config and
// confirming the fragment's own header reaches it alongside telemetry's
// traceparent/baggage, while the fragment's OWN colliding traceparent loses.
// This is the same check performed by hand against the live cluster on
// 2026-09-02; here it runs in CI on every push.
func TestPhase19EndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sink, bus, repo, repo)
	lifecycle := lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("honryu/jmeter:latest")).WithMetrics(collector)

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Scenarios:     scenarioapp.NewService(repo, store),
		Executions:    executionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycle,
		Reports:       repo,
		Metrics:       collector,
		Store:         store,
		DefaultOwners: []string{"honryu"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	scenarioID := postForm(t, client, srv.URL+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})

	// G5/G6/F4: validate an uncompiled key BEFORE ever storing it. Must
	// accept (valid:true), carry exactly one info diagnostic, and that
	// diagnostic must name the key plainly -- not "think-time not found in
	// type taurus.Scenario", the review-finding-5 leak.
	uncompiled := "think-time: 1.5s\nrequests:\n  - url: /a\n"
	diagBody := postRawBody(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/requests/validate",
		"text/yaml", uncompiled, http.StatusOK)
	var diagResp struct {
		Diagnostics []struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Line     int    `json:"line"`
		} `json:"diagnostics"`
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal([]byte(diagBody), &diagResp); err != nil {
		t.Fatalf("decode validate response %q: %v", diagBody, err)
	}
	if !diagResp.Valid || len(diagResp.Diagnostics) != 1 {
		t.Fatalf("validate(think-time) = %+v, want valid with exactly 1 info diagnostic", diagResp)
	}
	if d := diagResp.Diagnostics[0]; d.Severity != "info" {
		t.Errorf("diagnostic severity = %q, want info", d.Severity)
	} else if strings.Contains(d.Message, "taurus.") || strings.Contains(d.Message, "not found in type") {
		t.Errorf("diagnostic message %q leaks Go/yaml.v3 internals (review finding 5)", d.Message)
	} else if !strings.Contains(d.Message, "stored but not compiled") {
		t.Errorf("diagnostic message %q must say stored but not compiled", d.Message)
	}

	// G2/G3: store the real fragment as a raw text/yaml body (not
	// multipart), carrying the operator's own header and a traceparent that
	// must lose to telemetry's. Read back byte-exact.
	frag := "default-address: http://example.com\n" +
		"headers:\n" +
		"  X-Phase19-E2E: ship-gate\n" +
		"  traceparent: 00-fragment-must-lose-00000000000000-01\n" +
		"requests:\n" +
		"  - url: /checkout\n"
	putRaw(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/requests", "text/yaml", frag, http.StatusOK)
	if got := getBody(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/requests", http.StatusOK); got != frag {
		t.Fatalf("GET requests = %q, want byte-exact %q", got, frag)
	}

	// G7: the load profile as JSON, not the historical multipart multi-test
	// wrapper -- the shape R9's new-test flow posts.
	executionID := postForm(t, client, srv.URL+"/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	cfgJSON := fmt.Sprintf(
		`{"name":"peak","project_id":%d,"execution_id":%d,"tests":[{"name":"checkout","scenario_id":%d,"concurrency":1,"rampup":1,"engines":1,"duration":30}]}`,
		projectID, executionID, scenarioID)
	putRaw(t, client, srv.URL+"/api/executions/"+itoa(executionID)+"/config", "application/json", cfgJSON, http.StatusOK)

	base := srv.URL + "/api/executions/" + itoa(executionID)

	postAction(t, client, base+"/deploy", http.StatusOK)
	if st := getStatus(t, client, base+"/status"); st.Phase != run.PhaseDeployed || st.PoolSize != 1 {
		t.Fatalf("after deploy: %+v, want deployed/1", st)
	}

	postAction(t, client, base+"/trigger", http.StatusOK)
	if st := getStatus(t, client, base+"/status"); st.Phase != run.PhaseRunning {
		t.Fatalf("after trigger phase = %q, want running", st.Phase)
	}

	// Stop finalizes the report -- the only HTTP-visible way to learn the
	// run id snapshotConfigs used to key the compiled config
	// (POST /trigger's own response carries no id; see F5's task notes).
	postAction(t, client, base+"/stop", http.StatusOK)

	var reports []report.Report
	getJSON(t, client, base+"/reports", http.StatusOK, &reports)
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want exactly 1", len(reports))
	}
	rep := reports[0]
	if rep.ExecutionID != executionID {
		t.Errorf("report execution_id = %d, want %d", rep.ExecutionID, executionID)
	}
	if rep.RunID == 0 {
		t.Fatal("report run_id is 0 -- Trigger never started a run")
	}

	// G8 ship gate, for real: the ACTUAL compiled config the run used, read
	// back through the same endpoint the operator UI's engine-config view
	// hits -- not a unit test's in-memory taurus.Scenario struct.
	shardURL := fmt.Sprintf("%s/api/runs/%d/scenarios/%d/shards/0/config", srv.URL, rep.RunID, scenarioID)
	compiled := getBody(t, client, shardURL, http.StatusOK)
	if !strings.Contains(compiled, "X-Phase19-E2E: ship-gate") {
		t.Errorf("compiled config is missing the fragment's own header:\n%s", compiled)
	}
	if strings.Contains(compiled, "fragment-must-lose") {
		t.Errorf("compiled config still carries the fragment's own traceparent -- telemetry must win:\n%s", compiled)
	}
	if !strings.Contains(compiled, "traceparent:") || !strings.Contains(compiled, "baggage:") {
		t.Errorf("compiled config is missing telemetry's traceparent/baggage:\n%s", compiled)
	}

	postAction(t, client, base+"/purge", http.StatusOK)
}

// postRawBody sends a POST with a raw body (not multipart, not a form), the
// shape the requests-fragment validate endpoint expects, and returns the
// response body for the caller to decode.
func postRawBody(t *testing.T, client *http.Client, urlStr, contentType, body string, wantStatus int) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST %s body: %v", urlStr, err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s status = %d, want %d (body %s)", urlStr, resp.StatusCode, wantStatus, respBody)
	}
	return string(respBody)
}

// putRaw sends a PUT with a raw body (not multipart) -- G3's text/yaml
// fragment body and G7's application/json config body both need this; the
// existing putMultipart in phase1_e2e_test.go always wraps in multipart.
func putRaw(t *testing.T, client *http.Client, urlStr, contentType, body string, wantStatus int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, urlStr, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT %s status = %d, want %d (body %s)", urlStr, resp.StatusCode, wantStatus, respBody)
	}
}
