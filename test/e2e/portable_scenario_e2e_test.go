//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

// TestPortableScenarioEndToEnd drives task 23b's whole point: a scenario
// created with no uploaded artefact stays portable, cannot be deployed until
// its declarative requests are supplied, and runs once they are -- over real
// HTTP, with a real MySQL container behind it.
func TestPortableScenarioEndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Scenarios:     scenarioapp.NewService(repo, store),
		Executions:    executionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("honryu/jmeter:latest")),
		Store:         store,
		DefaultOwners: []string{"honryu"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	// No file upload at all -- this is what stays portable, per task 23b.
	scenarioID := postForm(t, client, srv.URL+"/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {itoa(projectID)}})

	collID := postForm(t, client, srv.URL+"/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	cfg := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 5\n      rampup: 1\n      engines: 1\n      duration: 30\n", collID, scenarioID)
	putMultipart(t, client, srv.URL+"/api/executions/"+itoa(collID)+"/config", "config.yaml", cfg)

	base := srv.URL + "/api/executions/" + itoa(collID)

	// Deploying before any requests are uploaded fails clearly (400, the same
	// class of error a native scenario missing its script would get), not a
	// 500 or a silent no-op deploy.
	postAction(t, client, base+"/deploy", http.StatusBadRequest)

	// Upload the declarative workload, then deploy succeeds.
	putMultipart(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/requests", "requests.yml",
		"default-address: http://example.com\nrequests:\n  - url: /checkout\n")

	postAction(t, client, base+"/deploy", http.StatusOK)
	if st := getStatus(t, client, base+"/status"); st.Phase != run.PhaseDeployed || st.PoolSize != 1 {
		t.Fatalf("after deploy: %+v, want deployed/1", st)
	}

	postAction(t, client, base+"/trigger", http.StatusOK)
	if st := getStatus(t, client, base+"/status"); st.Phase != run.PhaseRunning {
		t.Fatalf("after trigger phase = %q, want running", st.Phase)
	}

	postAction(t, client, base+"/stop", http.StatusOK)
	postAction(t, client, base+"/purge", http.StatusOK)
}
