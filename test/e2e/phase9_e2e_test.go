//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

type phase9Env struct {
	client *http.Client
	url    string
}

func setupPhase9(t *testing.T) *phase9Env {
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
	return &phase9Env{client: srv.Client(), url: srv.URL}
}

// configWithThroughput compiles a single-scenario config requesting a target
// QPS -- minimalConfig (phase6/8) never sets one, since it doesn't need to.
func configWithThroughput(executionID, scenarioID int64, throughput, duration int) string {
	return fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 1\n      rampup: 1\n      engines: 1\n      throughput: %d\n      duration: %d\n",
		executionID, scenarioID, throughput, duration)
}

// runOnce deploys, triggers, and finalizes one run of an execution via the
// real HTTP pipeline (the fake scheduler has no real engine, so there is no
// deploy/trigger race: DeployScenario just records the deployment
// synchronously). succeeded controls the achieved sample count -- with the
// wall-clock elapsed between trigger and finalize expected to round to under
// a second in this in-process test, achievedSeconds falls back to the
// configured duration, but the margins here (succeeded counts differing by
// orders of magnitude from the target) make the resulting achieved-QPS
// comfortably unambiguous even if real wall-clock time is used instead.
func runOnce(t *testing.T, e *phase9Env, executionID, scenarioID, runID, succeeded int64) {
	t.Helper()
	base := e.url + "/api/executions/" + itoa(executionID)
	postAction(t, e.client, base+"/deploy", http.StatusOK)
	postAction(t, e.client, base+"/trigger", http.StatusOK)
	ingestFinal(t, e.client, e.url, executionID, scenarioID, runID, 0, succeeded, 0, nil)
	postAction(t, e.client, base+"/purge", http.StatusOK)
}

func seedProjectScenarioExecution(t *testing.T, e *phase9Env, name string, tenantID, throughput, duration int) (projectID, scenarioID, executionID int64) {
	t.Helper()
	projectID = postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {name}, "owner": {"honryu"}, "tenant_id": {itoa(int64(tenantID))}})
	scenarioID = postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})
	putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioID)+"/files", "s.jmx", "<jmx/>")
	executionID = postForm(t, e.client, e.url+"/api/executions", url.Values{"name": {"readiness"}, "project_id": {itoa(projectID)}})
	putMultipart(t, e.client, e.url+"/api/executions/"+itoa(executionID)+"/config", "config.yaml", configWithThroughput(executionID, scenarioID, throughput, duration))
	return projectID, scenarioID, executionID
}

func TestPhase9_Analytics(t *testing.T) {
	e := setupPhase9(t)

	// A service that passes its criteria (no configured criteria to fail,
	// exit code 0) but whose run only achieves a small fraction of its
	// target QPS is not a real go: the campaign flips to no-go, naming the
	// shortfall.
	t.Run("QPSShortfallFlipsCampaignToNoGo", func(t *testing.T) {
		projectID, scenarioID, executionID := seedProjectScenarioExecution(t, e, "svc-a", 7, 100, 30)
		runOnce(t, e, executionID, scenarioID, 1, 1) // ~1/30 QPS, far under 100

		start := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		end := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		campaignID := postForm(t, e.client, e.url+"/api/tenants/7/campaigns", url.Values{
			"name": {"launch"}, "window_start": {start}, "window_end": {end},
			"service_project_id": {itoa(projectID)}, "service_execution_id": {itoa(executionID)},
		})

		var verdict struct {
			Go       bool `json:"go"`
			Services []struct {
				Outcome             string  `json:"outcome"`
				ShortOfTargetQPS    bool    `json:"short_of_target_qps"`
				RequestedThroughput float64 `json:"requested_throughput"`
				AchievedThroughput  float64 `json:"achieved_throughput"`
			} `json:"services"`
		}
		getJSON(t, e.client, e.url+"/api/campaigns/"+itoa(campaignID)+"/verdict", http.StatusOK, &verdict)
		if verdict.Go {
			t.Fatalf("verdict.go = true, want false (short of target QPS): %+v", verdict)
		}
		if len(verdict.Services) != 1 || verdict.Services[0].Outcome != "passed" {
			t.Fatalf("verdict services = %+v, want the passed outcome preserved", verdict.Services)
		}
		sv := verdict.Services[0]
		if !sv.ShortOfTargetQPS || sv.RequestedThroughput != 100 {
			t.Fatalf("service verdict = %+v, want short_of_target_qps naming a requested throughput of 100", sv)
		}
	})

	// A campaign compared to an older, ended campaign for the same tenant
	// classifies a shared project as improved: it missed target QPS in the
	// older campaign, and hits it now.
	t.Run("ComparisonClassifiesImprovedProject", func(t *testing.T) {
		const tenantID = 8
		projectID, scenarioOlder, execOlder := seedProjectScenarioExecution(t, e, "svc-b-older", tenantID, 100, 30)
		runOnce(t, e, execOlder, scenarioOlder, 2, 1) // far under target

		olderStart := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
		olderEnd := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
		olderCampaignID := postForm(t, e.client, e.url+"/api/tenants/"+itoa(tenantID)+"/campaigns", url.Values{
			"name": {"older"}, "window_start": {olderStart}, "window_end": {olderEnd},
			"service_project_id": {itoa(projectID)}, "service_execution_id": {itoa(execOlder)},
		})

		// A fresh execution for the same project -- a rerun, as a later
		// campaign designates in practice -- that comfortably hits target.
		scenarioNewer := postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout-2"}, "project_id": {itoa(projectID)}})
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioNewer)+"/files", "s.jmx", "<jmx/>")
		execNewer := postForm(t, e.client, e.url+"/api/executions", url.Values{"name": {"readiness-2"}, "project_id": {itoa(projectID)}})
		putMultipart(t, e.client, e.url+"/api/executions/"+itoa(execNewer)+"/config", "config.yaml", configWithThroughput(execNewer, scenarioNewer, 100, 30))
		runOnce(t, e, execNewer, scenarioNewer, 3, 100000) // far over target

		targetStart := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		targetEnd := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		targetCampaignID := postForm(t, e.client, e.url+"/api/tenants/"+itoa(tenantID)+"/campaigns", url.Values{
			"name": {"target"}, "window_start": {targetStart}, "window_end": {targetEnd},
			"service_project_id": {itoa(projectID)}, "service_execution_id": {itoa(execNewer)},
		})

		var comparison struct {
			HasBaseline        bool  `json:"has_baseline"`
			BaselineCampaignID int64 `json:"baseline_campaign_id"`
			Services           []struct {
				ProjectID int64  `json:"project_id"`
				Status    string `json:"status"`
			} `json:"services"`
		}
		getJSON(t, e.client, e.url+"/api/campaigns/"+itoa(targetCampaignID)+"/comparison", http.StatusOK, &comparison)
		if !comparison.HasBaseline || comparison.BaselineCampaignID != olderCampaignID {
			t.Fatalf("comparison = %+v, want the older campaign (%d) as the resolved baseline", comparison, olderCampaignID)
		}
		if len(comparison.Services) != 1 || comparison.Services[0].ProjectID != projectID || comparison.Services[0].Status != "improved" {
			t.Fatalf("comparison services = %+v, want project %d classified improved", comparison.Services, projectID)
		}
	})

	// A per-service trend returns the execution's run series, most recent
	// first, through the real deploy/trigger/ingest pipeline.
	t.Run("TrendReturnsRunSeries", func(t *testing.T) {
		_, scenarioID, executionID := seedProjectScenarioExecution(t, e, "svc-c", 9, 100, 30)
		runOnce(t, e, executionID, scenarioID, 4, 1)      // missed target
		runOnce(t, e, executionID, scenarioID, 5, 100000) // hit target

		var trend struct {
			ExecutionID int64 `json:"execution_id"`
			Points      []struct {
				RunID        int64 `json:"run_id"`
				HitTargetQPS bool  `json:"hit_target_qps"`
			} `json:"points"`
		}
		getJSON(t, e.client, e.url+"/api/executions/"+itoa(executionID)+"/trend", http.StatusOK, &trend)
		if trend.ExecutionID != executionID || len(trend.Points) != 2 {
			t.Fatalf("trend = %+v, want execution_id %d and 2 points", trend, executionID)
		}
		if trend.Points[0].RunID != 5 || !trend.Points[0].HitTargetQPS {
			t.Fatalf("trend.Points[0] = %+v, want run 5 (most recent) hitting target", trend.Points[0])
		}
		if trend.Points[1].RunID != 4 || trend.Points[1].HitTargetQPS {
			t.Fatalf("trend.Points[1] = %+v, want run 4 missing target", trend.Points[1])
		}
	})
}
