//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/internal/adapters/storage/local"
	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/internal/domain/run"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

// TestPhase2_LifecycleEndToEnd drives deploy → trigger → stop → purge over real
// HTTP, with run state persisted in a real MySQL container. The scheduler and
// executor are the in-memory fakes (a real kind+JMeter run is a nightly concern
// covered by the k8s/jmeter adapter tests); this exercises the full control
// plane and its persistence.
func TestPhase2_LifecycleEndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	exec := fake.NewExecutor()

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Scenarios:     scenarioapp.NewService(repo, store),
		Executions:    executionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycleapp.NewService(repo, sched, exec, store, "honryu/jmeter:latest"),
		Store:         store,
		DefaultOwners: []string{"honryu"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	scenarioID := postForm(t, client, srv.URL+"/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	putMultipart(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/files", "plan.jmx", "<jmx/>")
	collID := postForm(t, client, srv.URL+"/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	cfg := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 10\n      rampup: 1\n      engines: 2\n      duration: 30\n", collID, scenarioID)
	putMultipart(t, client, srv.URL+"/api/executions/"+itoa(collID)+"/config", "config.yaml", cfg)

	base := srv.URL + "/api/executions/" + itoa(collID)

	postAction(t, client, base+"/deploy", http.StatusOK)
	if st := getStatus(t, client, base+"/status"); st.Phase != run.PhaseDeployed || st.PoolSize != 2 {
		t.Fatalf("after deploy: %+v, want deployed/2", st)
	}

	postAction(t, client, base+"/trigger", http.StatusOK)
	st := getStatus(t, client, base+"/status")
	if st.Phase != run.PhaseRunning {
		t.Fatalf("after trigger phase = %q, want running", st.Phase)
	}
	if len(st.Scenarios) != 1 || !st.Scenarios[0].InProgress {
		t.Fatalf("after trigger plan not in progress: %+v", st.Scenarios)
	}
	if exec.TriggerCount() != 2 {
		t.Fatalf("triggered engines = %d, want 2", exec.TriggerCount())
	}

	// Triggering again while running is a conflict.
	postAction(t, client, base+"/trigger", http.StatusConflict)

	postAction(t, client, base+"/stop", http.StatusOK)
	if st := getStatus(t, client, base+"/status"); st.Phase == run.PhaseRunning {
		t.Fatalf("after stop still running: %+v", st)
	}

	postAction(t, client, base+"/purge", http.StatusOK)
}

func postAction(t *testing.T, client *http.Client, urlStr string, wantStatus int) {
	t.Helper()
	resp, err := client.Post(urlStr, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s status = %d, want %d", urlStr, resp.StatusCode, wantStatus)
	}
}

func getStatus(t *testing.T, client *http.Client, urlStr string) lifecycleapp.Status {
	t.Helper()
	resp, err := client.Get(urlStr)
	if err != nil {
		t.Fatalf("GET %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", urlStr, resp.StatusCode)
	}
	var st lifecycleapp.Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return st
}
