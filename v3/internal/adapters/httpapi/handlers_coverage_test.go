package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/v3/internal/app/collectionapp"
	"github.com/heridotlife/Setagaya/v3/internal/app/planapp"
	"github.com/heridotlife/Setagaya/v3/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/domain/plan"
	"github.com/heridotlife/Setagaya/v3/internal/domain/project"
	"github.com/heridotlife/Setagaya/v3/internal/ports/fake"
)

func routerWithStore(t *testing.T) (http.Handler, *fake.Store, *fake.ObjectStore) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Plans:         planapp.NewService(store, obj),
		Collections:   collectionapp.NewService(store, obj, 100),
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
	planID := decodeID(t, postForm(t, h, "/api/plans", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}}))
	collID := decodeID(t, postForm(t, h, "/api/collections", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))

	// Plan data file: upload, list, delete.
	putMultipart(t, h, "/api/plans/"+itoa(planID)+"/files", "users.csv", "a,b")
	if rec := do(t, h, http.MethodGet, "/api/plans/"+itoa(planID)+"/files"); rec.Code != http.StatusOK {
		t.Fatalf("list plan files = %d", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/plans/"+itoa(planID)+"/files", url.Values{"filename": {"users.csv"}}); rec.Code != http.StatusOK {
		t.Fatalf("delete plan file = %d (%s)", rec.Code, rec.Body.String())
	}

	// Collection data file: upload, list, delete.
	putMultipart(t, h, "/api/collections/"+itoa(collID)+"/files", "shared.csv", "x,y")
	if rec := do(t, h, http.MethodGet, "/api/collections/"+itoa(collID)+"/files"); rec.Code != http.StatusOK {
		t.Fatalf("list collection files = %d", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/collections/"+itoa(collID)+"/files", url.Values{"filename": {"shared.csv"}}); rec.Code != http.StatusOK {
		t.Fatalf("delete collection file = %d", rec.Code)
	}

	// Empty config get, then delete collection, plan, and empty project.
	if rec := do(t, h, http.MethodGet, "/api/collections/"+itoa(collID)+"/config"); rec.Code != http.StatusOK {
		t.Fatalf("get config = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/collections/"+itoa(collID)); rec.Code != http.StatusOK {
		t.Fatalf("delete collection = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/plans/"+itoa(planID)); rec.Code != http.StatusOK {
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
		{http.MethodGet, "/api/plans/x"},
		{http.MethodDelete, "/api/plans/x"},
		{http.MethodGet, "/api/plans/x/files"},
		{http.MethodPut, "/api/plans/x/files"},
		{http.MethodDelete, "/api/plans/x/files"},
		{http.MethodGet, "/api/collections/x"},
		{http.MethodDelete, "/api/collections/x"},
		{http.MethodGet, "/api/collections/x/files"},
		{http.MethodPut, "/api/collections/x/files"},
		{http.MethodDelete, "/api/collections/x/files"},
		{http.MethodPut, "/api/collections/x/config"},
		{http.MethodGet, "/api/collections/x/config"},
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

	if rec := do(t, h, http.MethodGet, "/api/plans/999"); rec.Code != http.StatusNotFound {
		t.Errorf("get missing plan = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/collections/999"); rec.Code != http.StatusNotFound {
		t.Errorf("get missing collection = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/plans/999"); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing plan = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/files/plan/1/missing.jmx"); rec.Code != http.StatusNotFound {
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
	foreignPlan, _ := plan.New("p", projectID)
	planID, _ := store.CreatePlan(ctx, foreignPlan)
	foreignColl, _ := collection.New("c", projectID)
	collID, _ := store.CreateCollection(ctx, foreignColl)

	checks := []struct{ method, path string }{
		{http.MethodDelete, "/api/projects/" + itoa(projectID)},
		{http.MethodDelete, "/api/plans/" + itoa(planID)},
		{http.MethodDelete, "/api/collections/" + itoa(collID)},
	}
	for _, c := range checks {
		if rec := do(t, h, c.method, c.path); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c.method, c.path, rec.Code)
		}
	}

	// Creating a plan/collection under a foreign project is also forbidden.
	if rec := postForm(t, h, "/api/plans", url.Values{"name": {"x"}, "project_id": {itoa(projectID)}}); rec.Code != http.StatusForbidden {
		t.Errorf("create plan under foreign project = %d, want 403", rec.Code)
	}
	if rec := postForm(t, h, "/api/collections", url.Values{"name": {"x"}, "project_id": {itoa(projectID)}}); rec.Code != http.StatusForbidden {
		t.Errorf("create collection under foreign project = %d, want 403", rec.Code)
	}
}

func TestHandlers_BadFormBodies(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// Non-numeric project_id on create plan/collection → 400.
	if rec := postForm(t, h, "/api/plans", url.Values{"name": {"x"}, "project_id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create plan bad project_id = %d, want 400", rec.Code)
	}
	if rec := postForm(t, h, "/api/collections", url.Values{"name": {"x"}, "project_id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create collection bad project_id = %d, want 400", rec.Code)
	}

	// Invalid YAML config upload → 400.
	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}}))
	collID := decodeID(t, postForm(t, h, "/api/collections", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))
	if rec := putMultipart(t, h, "/api/collections/"+itoa(collID)+"/config", "c.yaml", "a: b: c"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid yaml = %d, want 400", rec.Code)
	}
}
