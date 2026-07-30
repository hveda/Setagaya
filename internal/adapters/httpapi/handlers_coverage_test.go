package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

func routerWithStore(t *testing.T) (http.Handler, *fake.Store, *fake.ObjectStore) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Plans:         scenarioapp.NewService(store, obj),
		Collections:   executionapp.NewService(store, obj, 100),
		Store:         obj,
		DefaultOwners: []string{"setagaya"},
	})
	return router, store, obj
}

func deleteWithQuery(t *testing.T, h http.Handler, path string, query url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path+"?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestFileEndpointsHappyPath exercises the file-listing and deletion handlers
// that the main flow test does not reach.
func TestFileEndpointsHappyPath(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}}))
	collID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))

	// Plan data file: upload, list, delete.
	putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", "users.csv", "a,b")
	if rec := do(t, h, http.MethodGet, "/api/scenarios/"+itoa(scenarioID)+"/files"); rec.Code != http.StatusOK {
		t.Fatalf("list plan files = %d", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", url.Values{"filename": {"users.csv"}}); rec.Code != http.StatusOK {
		t.Fatalf("delete plan file = %d (%s)", rec.Code, rec.Body.String())
	}

	// Collection data file: upload, list, delete.
	putMultipart(t, h, "/api/executions/"+itoa(collID)+"/files", "shared.csv", "x,y")
	if rec := do(t, h, http.MethodGet, "/api/executions/"+itoa(collID)+"/files"); rec.Code != http.StatusOK {
		t.Fatalf("list collection files = %d", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/executions/"+itoa(collID)+"/files", url.Values{"filename": {"shared.csv"}}); rec.Code != http.StatusOK {
		t.Fatalf("delete collection file = %d", rec.Code)
	}

	// Empty config get, then delete collection, plan, and empty project.
	if rec := do(t, h, http.MethodGet, "/api/executions/"+itoa(collID)+"/config"); rec.Code != http.StatusOK {
		t.Fatalf("get config = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/executions/"+itoa(collID)); rec.Code != http.StatusOK {
		t.Fatalf("delete collection = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/scenarios/"+itoa(scenarioID)); rec.Code != http.StatusOK {
		t.Fatalf("delete plan = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/projects/"+itoa(projectID)); rec.Code != http.StatusOK {
		t.Fatalf("delete empty project = %d", rec.Code)
	}
}

func TestHandlers_InvalidID_400(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/projects/x"},
		{http.MethodDelete, "/api/projects/x"},
		{http.MethodGet, "/api/scenarios/x"},
		{http.MethodDelete, "/api/scenarios/x"},
		{http.MethodGet, "/api/scenarios/x/files"},
		{http.MethodPut, "/api/scenarios/x/files"},
		{http.MethodDelete, "/api/scenarios/x/files"},
		{http.MethodGet, "/api/executions/x"},
		{http.MethodDelete, "/api/executions/x"},
		{http.MethodGet, "/api/executions/x/files"},
		{http.MethodPut, "/api/executions/x/files"},
		{http.MethodDelete, "/api/executions/x/files"},
		{http.MethodPut, "/api/executions/x/config"},
		{http.MethodGet, "/api/executions/x/config"},
	}
	for _, tc := range cases {
		if rec := do(t, h, tc.method, tc.path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tc.method, tc.path, rec.Code)
		}
	}
}

func TestHandlers_NotFound_404(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	if rec := do(t, h, http.MethodGet, "/api/scenarios/999"); rec.Code != http.StatusNotFound {
		t.Errorf("get missing plan = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/executions/999"); rec.Code != http.StatusNotFound {
		t.Errorf("get missing collection = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/scenarios/999"); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing plan = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/files/scenario/1/missing.jmx"); rec.Code != http.StatusNotFound {
		t.Errorf("download missing file = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/files/bogus/1/x"); rec.Code != http.StatusBadRequest {
		t.Errorf("download bad kind = %d, want 400", rec.Code)
	}
}

// TestHandlers_Forbidden_403 seeds resources owned by a different account and
// verifies mutations are rejected.
func TestHandlers_Forbidden_403(t *testing.T) {
	t.Parallel()
	h, store, _ := routerWithStore(t)
	ctx := context.Background()

	foreignProject, _ := project.New("secret", "other-team", "")
	projectID, _ := store.CreateProject(ctx, foreignProject)
	foreignScenario, _ := scenario.New("p", projectID)
	scenarioID, _ := store.CreateScenario(ctx, foreignScenario)
	foreignColl, _ := execution.New("c", projectID)
	collID, _ := store.CreateExecution(ctx, foreignColl)

	checks := []struct{ method, path string }{
		{http.MethodDelete, "/api/projects/" + itoa(projectID)},
		{http.MethodDelete, "/api/scenarios/" + itoa(scenarioID)},
		{http.MethodDelete, "/api/executions/" + itoa(collID)},
	}
	for _, c := range checks {
		if rec := do(t, h, c.method, c.path); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, rec.Code)
		}
	}

	// Creating a plan/collection under a foreign project is also forbidden.
	if rec := postForm(t, h, "/api/scenarios", url.Values{"name": {"x"}, "project_id": {itoa(projectID)}}); rec.Code != http.StatusForbidden {
		t.Errorf("create plan under foreign project = %d, want 403", rec.Code)
	}
	if rec := postForm(t, h, "/api/executions", url.Values{"name": {"x"}, "project_id": {itoa(projectID)}}); rec.Code != http.StatusForbidden {
		t.Errorf("create collection under foreign project = %d, want 403", rec.Code)
	}
}

func TestHandlers_BadFormBodies(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// Non-numeric project_id on create plan/collection → 400.
	if rec := postForm(t, h, "/api/scenarios", url.Values{"name": {"x"}, "project_id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create plan bad project_id = %d, want 400", rec.Code)
	}
	if rec := postForm(t, h, "/api/executions", url.Values{"name": {"x"}, "project_id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create collection bad project_id = %d, want 400", rec.Code)
	}

	// Invalid YAML config upload → 400.
	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}}))
	collID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))
	if rec := putMultipart(t, h, "/api/executions/"+itoa(collID)+"/config", "c.yaml", "a: b: c"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid yaml = %d, want 400", rec.Code)
	}
}
