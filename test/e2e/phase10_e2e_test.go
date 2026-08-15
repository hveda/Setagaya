//go:build e2e

package e2e_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
	"gopkg.in/yaml.v3"
)

type phase10Env struct {
	client *http.Client
	url    string
	repo   *mysqladapter.Repository
}

func setupPhase10(t *testing.T) *phase10Env {
	t.Helper()
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sink, bus, repo, repo)
	campaigns := campaignapp.NewService(repo, sched)
	lifecycle := lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("jmeter")).WithMetrics(collector)

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Scenarios:     scenarioapp.NewService(repo, store),
		Executions:    executionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycle,
		Campaigns:     campaigns,
		Store:         store,
		Metrics:       collector,
		Reports:       repo,
		IngestToken:   "engine-token",
		DefaultOwners: []string{"honryu"},
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &phase10Env{client: srv.Client(), url: srv.URL, repo: repo}
}

// traceparentWellFormed matches the exact shape telemetry.Headers renders:
// version 00, a 32-hex trace id, a 16-hex parent id, sampled flag always 00.
var traceparentWellFormed = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-00$`)

// shardHeaders fetches a run's compiled shard config over HTTP and returns the
// scenario-level headers it carries. The config is the exact YAML the run's
// engine pods were handed, snapshotted at Trigger so a later deploy cannot
// rewrite it -- which is what makes it evidence of what the load carried.
func shardHeaders(t *testing.T, e *phase10Env, runID, scenarioID int64) map[string]string {
	t.Helper()
	resp, err := e.client.Get(e.url + "/api/runs/" + itoa(runID) + "/scenarios/" + itoa(scenarioID) + "/shards/0/config")
	if err != nil {
		t.Fatalf("GET shard config: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET shard config status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read shard config: %v", err)
	}

	var cfg struct {
		Scenarios map[string]struct {
			Headers map[string]string `yaml:"headers"`
		} `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse shard config: %v\n%s", err, raw)
	}
	if len(cfg.Scenarios) != 1 {
		t.Fatalf("shard config has %d scenarios, want exactly 1:\n%s", len(cfg.Scenarios), raw)
	}
	for name, sc := range cfg.Scenarios {
		if len(sc.Headers) == 0 {
			t.Fatalf("scenario %q carries no headers -- the deploy's trace context never reached the compiled config:\n%s", name, raw)
		}
		return sc.Headers
	}
	return nil
}

// baggageMap splits a baggage header into its entries, so a test can assert
// individual keys without matching the whole joined string.
func baggageMap(t *testing.T, bg string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, entry := range strings.Split(bg, ",") {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("baggage entry %q is not key=value (baggage was %q)", entry, bg)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// One deploy/trigger/finalize cycle of an execution. Returns the run id and
// the trace id that run's generated load carried, after asserting the full
// correlation chain end to end: well-formed headers on the compiled config,
// baggage naming this tenant/project/execution/run, and a finalized report
// whose correlation id is exactly that trace id.
func correlatedCycle(t *testing.T, e *phase10Env, tenantID, projectID, executionID, scenarioID int64) (runID int64, traceID string) {
	t.Helper()
	ctx := context.Background()
	base := e.url + "/api/executions/" + itoa(executionID)
	postAction(t, e.client, base+"/deploy", http.StatusOK)
	postAction(t, e.client, base+"/trigger", http.StatusOK)

	runID, running, err := e.repo.CurrentRun(ctx, executionID)
	if err != nil || !running {
		t.Fatalf("CurrentRun after trigger: run %d, running %v, err %v", runID, running, err)
	}

	headers := shardHeaders(t, e, runID, scenarioID)
	m := traceparentWellFormed.FindStringSubmatch(headers["traceparent"])
	if m == nil {
		t.Fatalf("traceparent = %q, want 00-<32 hex>-<16 hex>-00", headers["traceparent"])
	}
	traceID = m[1]

	bag := baggageMap(t, headers["baggage"])
	want := map[string]string{
		"honryu.tenant":    itoa(tenantID),
		"honryu.service":   itoa(projectID),
		"honryu.execution": itoa(executionID),
		"honryu.run":       traceID,
	}
	for k, v := range want {
		if bag[k] != v {
			t.Fatalf("baggage %s = %q, want %q (baggage was %q)", k, bag[k], v, headers["baggage"])
		}
	}

	ingestFinal(t, e.client, e.url, executionID, scenarioID, runID, 0, 10, 0, nil)
	var rep struct {
		CorrelationID string `json:"correlation_id"`
	}
	getJSON(t, e.client, e.url+"/api/runs/"+itoa(runID)+"/report", http.StatusOK, &rep)
	if rep.CorrelationID != traceID {
		t.Fatalf("run %d report correlation id = %q, want the config's trace id %q", runID, rep.CorrelationID, traceID)
	}

	postAction(t, e.client, base+"/purge", http.StatusOK)
	return runID, traceID
}

// The correlation chain, end to end: a deploy mints a trace context, every
// shard's compiled config carries it as well-formed traceparent/baggage, the
// run's finalized report surfaces the same id, and a second deploy of the same
// execution mints a fresh one -- proving the id is per-deploy, not a stable
// property of the execution -- while the first run's report keeps the id its
// own load actually carried.
func TestPhase10_CorrelationIDPerDeploy(t *testing.T) {
	e := setupPhase10(t)
	ctx := context.Background()

	projectID := postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {"telemetry"}, "owner": {"honryu"}})
	scenarioID := postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})
	putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioID)+"/files", "s.jmx", "<jmx/>")

	// Repo-seed the execution with a tenant (the create API does not populate
	// one, phase 8's pattern) so the baggage assertion covers all four
	// honryu.* entries including honryu.tenant.
	tenantID := int64(42)
	exe, err := execution.New("correlated", projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	exe.TenantID = &tenantID
	executionID, err := e.repo.CreateExecution(ctx, exe)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	putMultipart(t, e.client, e.url+"/api/executions/"+itoa(executionID)+"/config", "config.yaml", minimalConfig(executionID, scenarioID, ""))

	run1, trace1 := correlatedCycle(t, e, tenantID, projectID, executionID, scenarioID)
	run2, trace2 := correlatedCycle(t, e, tenantID, projectID, executionID, scenarioID)

	if trace1 == trace2 {
		t.Fatalf("both deploys minted the same trace id %q -- the id is stable per execution, not fresh per deploy", trace1)
	}

	// The first run's report still names the id its own load carried, not the
	// second deploy's: reports are per-run evidence, and the pending id on the
	// execution has since moved on.
	var rep1 struct {
		CorrelationID string `json:"correlation_id"`
	}
	getJSON(t, e.client, e.url+"/api/runs/"+itoa(run1)+"/report", http.StatusOK, &rep1)
	if rep1.CorrelationID != trace1 {
		t.Fatalf("run %d report correlation id = %q after the second deploy, want its own %q", run1, rep1.CorrelationID, trace1)
	}
	var rep2 struct {
		CorrelationID string `json:"correlation_id"`
	}
	getJSON(t, e.client, e.url+"/api/runs/"+itoa(run2)+"/report", http.StatusOK, &rep2)
	if rep2.CorrelationID != trace2 {
		t.Fatalf("run %d report correlation id = %q, want its own %q", run2, rep2.CorrelationID, trace2)
	}
}
