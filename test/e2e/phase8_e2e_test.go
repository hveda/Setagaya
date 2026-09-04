//go:build e2e

package e2e_test

import (
	"context"
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
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

type phase8Env struct {
	client *http.Client
	url    string
	repo   *mysqladapter.Repository
	sched  *fake.Scheduler
}

func setupPhase8(t *testing.T) *phase8Env {
	t.Helper()
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sink, bus, repo, repo)
	quota := quotaapp.NewService(repo)
	campaigns := campaignapp.NewService(repo, sched)
	lifecycle := lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("jmeter")).
		WithMetrics(collector).WithQuota(quota).WithFreeze(campaigns)

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
	return &phase8Env{client: srv.Client(), url: srv.URL, repo: repo, sched: sched}
}

// registerCluster stores a registry entry directly (the fake scheduler ignores
// the registry, so routing is proven by the DeploySpec the execution carries).
func (e *phase8Env) registerCluster(t *testing.T, name string) {
	t.Helper()
	if err := e.repo.CreateCluster(context.Background(), clusterregistry.Cluster{
		Name: name, APIURL: "https://" + name + ":6443", CACert: "ca", IngestURL: "http://ingest",
		SidecarImage: "sidecar:1", Namespace: "honryu", SecretRef: name + "-creds", Origin: clusterregistry.OriginOperator,
		CreatedBy: "admin", CreatedTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateCluster(%s): %v", name, err)
	}
}

func TestPhase8_MultiCluster(t *testing.T) {
	e := setupPhase8(t)
	ctx := context.Background()
	e.registerCluster(t, "cluster-a")
	e.registerCluster(t, "cluster-b")

	// An execution naming cluster-b deploys to cluster-b, and (with a tenant)
	// reserves quota against cluster-b's ledger -- not the default's.
	t.Run("ExecutionOnClusterBRoutesToB", func(t *testing.T) {
		projectID := postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {"svc"}, "owner": {"honryu"}})
		scenarioID := postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioID)+"/files", "s.jmx", "<jmx/>")

		// Repo-seed the execution with a tenant + cluster, pinning the exact
		// tenant id (the create API has stamped the project's tenant since
		// Phase 20 Block A), then drive the rest over HTTP.
		tenantID := int64(42)
		exe, _ := execution.New("on-b", projectID)
		exe.TenantID = &tenantID
		exe.Cluster = "cluster-b"
		executionID, err := e.repo.CreateExecution(ctx, exe)
		if err != nil {
			t.Fatalf("CreateExecution: %v", err)
		}
		if err := e.repo.SetCeiling(ctx, tenantID, "cluster-b", 5); err != nil {
			t.Fatalf("SetCeiling: %v", err)
		}
		putMultipart(t, e.client, e.url+"/api/executions/"+itoa(executionID)+"/config", "config.yaml", minimalConfig(executionID, scenarioID, ""))

		base := e.url + "/api/executions/" + itoa(executionID)
		postAction(t, e.client, base+"/deploy", http.StatusOK)
		postAction(t, e.client, base+"/trigger", http.StatusOK)

		// Deployed to cluster-b.
		spec, ok := e.sched.LastDeploy(executionID, scenarioID)
		if !ok || spec.Cluster != "cluster-b" {
			t.Fatalf("DeploySpec.Cluster = %q (ok=%v), want cluster-b", spec.Cluster, ok)
		}
		// Quota reserved on cluster-b, nothing on the default ledger.
		from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
		onB, err := e.repo.ReservationsInWindow(ctx, tenantID, "cluster-b", from, to)
		if err != nil {
			t.Fatalf("ReservationsInWindow(cluster-b): %v", err)
		}
		if len(onB) != 1 || onB[0].ExecutionID != executionID {
			t.Fatalf("reservations on cluster-b = %+v, want one for this execution", onB)
		}
		onDefault, err := e.repo.ReservationsInWindow(ctx, tenantID, "", from, to)
		if err != nil {
			t.Fatalf("ReservationsInWindow(default): %v", err)
		}
		if len(onDefault) != 0 {
			t.Fatalf("reservations on default = %+v, want none", onDefault)
		}
	})

	// A campaign whose two designated executions run on different clusters
	// still rolls up into one verdict -- the rollup is cluster-agnostic.
	t.Run("CampaignSpansClustersYieldsOneVerdict", func(t *testing.T) {
		// Phase 20 Block A stamps an HTTP-created execution with its
		// project's tenant, so triggering these tenant-9 executions consults
		// quota -- and a tenant with no configured ceiling reserves nothing
		// (0028's contract: "never an accidental unlimited default"). Set a
		// ceiling on each cluster, as the sibling subtest does for its own.
		if err := e.repo.SetCeiling(ctx, 9, "cluster-a", 5); err != nil {
			t.Fatalf("SetCeiling(cluster-a): %v", err)
		}
		if err := e.repo.SetCeiling(ctx, 9, "cluster-b", 5); err != nil {
			t.Fatalf("SetCeiling(cluster-b): %v", err)
		}
		projectA := postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {"span-a"}, "owner": {"honryu"}, "tenant_id": {"9"}})
		scenarioA := postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectA)}})
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioA)+"/files", "s.jmx", "<jmx/>")
		execA := postForm(t, e.client, e.url+"/api/executions", url.Values{"name": {"readiness"}, "project_id": {itoa(projectA)}, "cluster": {"cluster-a"}})
		putMultipart(t, e.client, e.url+"/api/executions/"+itoa(execA)+"/config", "config.yaml", minimalConfig(execA, scenarioA, ""))

		projectB := postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {"span-b"}, "owner": {"honryu"}, "tenant_id": {"9"}})
		scenarioB := postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectB)}})
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioB)+"/files", "s.jmx", "<jmx/>")
		execB := postForm(t, e.client, e.url+"/api/executions", url.Values{"name": {"readiness"}, "project_id": {itoa(projectB)}, "cluster": {"cluster-b"}})
		putMultipart(t, e.client, e.url+"/api/executions/"+itoa(execB)+"/config", "config.yaml", minimalConfig(execB, scenarioB, ""))

		windowStart := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		windowEnd := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		campaignID := postForm(t, e.client, e.url+"/api/tenants/9/campaigns", url.Values{
			"name":                 {"Cross-cluster Readiness"},
			"window_start":         {windowStart},
			"window_end":           {windowEnd},
			"service_project_id":   {itoa(projectA), itoa(projectB)},
			"service_execution_id": {itoa(execA), itoa(execB)},
		})

		baseA := e.url + "/api/executions/" + itoa(execA)
		postAction(t, e.client, baseA+"/deploy", http.StatusOK)
		postAction(t, e.client, baseA+"/trigger", http.StatusOK)
		runA, running, err := e.repo.CurrentRun(ctx, execA)
		if err != nil || !running {
			t.Fatalf("execA CurrentRun: %d, %v, %v", runA, running, err)
		}
		baseB := e.url + "/api/executions/" + itoa(execB)
		postAction(t, e.client, baseB+"/deploy", http.StatusOK)
		postAction(t, e.client, baseB+"/trigger", http.StatusOK)
		runB, running, err := e.repo.CurrentRun(ctx, execB)
		if err != nil || !running {
			t.Fatalf("execB CurrentRun: %d, %v, %v", runB, running, err)
		}

		// The two ran on different clusters.
		specA, _ := e.sched.LastDeploy(execA, scenarioA)
		specB, _ := e.sched.LastDeploy(execB, scenarioB)
		if specA.Cluster != "cluster-a" || specB.Cluster != "cluster-b" {
			t.Fatalf("spanning deploy clusters = %q / %q, want cluster-a / cluster-b", specA.Cluster, specB.Cluster)
		}

		ingestFinal(t, e.client, e.url, execA, scenarioA, runA, 0, 10, 0, nil)
		ingestFinal(t, e.client, e.url, execB, scenarioB, runB, 0, 10, 0, nil)

		var verdict struct {
			CampaignID int64 `json:"campaign_id"`
			Go         bool  `json:"go"`
			Services   []struct {
				ExecutionID int64  `json:"execution_id"`
				HasReport   bool   `json:"has_report"`
				Outcome     string `json:"outcome"`
			} `json:"services"`
		}
		getJSON(t, e.client, e.url+"/api/campaigns/"+itoa(campaignID)+"/verdict", http.StatusOK, &verdict)
		if verdict.CampaignID != campaignID || len(verdict.Services) != 2 {
			t.Fatalf("verdict = %+v, want one verdict over 2 services", verdict)
		}
		for _, s := range verdict.Services {
			if !s.HasReport {
				t.Fatalf("service %d has no report -- a cross-cluster run was dropped from the rollup", s.ExecutionID)
			}
		}
		if !verdict.Go {
			t.Fatalf("verdict.go = false, want true -- both cross-cluster runs passed: %+v", verdict)
		}

		// The report records each run's load origin.
		var repA struct {
			Cluster string `json:"cluster"`
		}
		getJSON(t, e.client, e.url+"/api/runs/"+itoa(runA)+"/report", http.StatusOK, &repA)
		if repA.Cluster != "cluster-a" {
			t.Fatalf("run A report cluster = %q, want cluster-a", repA.Cluster)
		}
	})

	// An execution with no cluster deploys to the default (empty ClusterRef),
	// exactly as before Phase 8.
	t.Run("EmptyClusterIsDefault", func(t *testing.T) {
		projectID := postForm(t, e.client, e.url+"/api/projects", url.Values{"name": {"legacy"}, "owner": {"honryu"}})
		scenarioID := postForm(t, e.client, e.url+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})
		putMultipart(t, e.client, e.url+"/api/scenarios/"+itoa(scenarioID)+"/files", "s.jmx", "<jmx/>")
		executionID := postForm(t, e.client, e.url+"/api/executions", url.Values{"name": {"no-cluster"}, "project_id": {itoa(projectID)}})
		putMultipart(t, e.client, e.url+"/api/executions/"+itoa(executionID)+"/config", "config.yaml", minimalConfig(executionID, scenarioID, ""))

		base := e.url + "/api/executions/" + itoa(executionID)
		postAction(t, e.client, base+"/deploy", http.StatusOK)

		spec, ok := e.sched.LastDeploy(executionID, scenarioID)
		if !ok || spec.Cluster != "" {
			t.Fatalf("DeploySpec.Cluster = %q (ok=%v), want empty (default)", spec.Cluster, ok)
		}
		// The execution response omits the cluster (default).
		var got struct {
			Cluster string `json:"cluster"`
		}
		getJSON(t, e.client, base, http.StatusOK, &got)
		if got.Cluster != "" {
			t.Fatalf("execution response cluster = %q, want empty", got.Cluster)
		}
	})
}
