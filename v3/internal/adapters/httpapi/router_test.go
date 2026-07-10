package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/v3/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/v3/internal/domain/project"
	"github.com/heridotlife/Setagaya/v3/internal/ports/fake"
)

// newTestRouter builds a router backed by an in-memory repo, optionally
// seeded, and returns the router plus the seeded project IDs.
func newTestRouter(t *testing.T, seed ...project.Project) (http.Handler, []int64) {
	t.Helper()
	repo := fake.NewProjectRepository()
	ids := make([]int64, 0, len(seed))
	for _, p := range seed {
		id, err := repo.CreateProject(context.Background(), p)
		if err != nil {
			t.Fatalf("seed CreateProject: %v", err)
		}
		ids = append(ids, id)
	}
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		DefaultOwners: []string{"setagaya"},
	})
	return router, ids
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	t.Parallel()
	router, _ := newTestRouter(t)

	rec := do(t, router, http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, rec.Body.String())
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()
	router, _ := newTestRouter(t)

	rec := do(t, router, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "# HELP") {
		t.Errorf("metrics body missing Prometheus exposition, got:\n%s", rec.Body.String())
	}
}

func TestListProjects_Empty(t *testing.T) {
	t.Parallel()
	router, _ := newTestRouter(t)

	rec := do(t, router, http.MethodGet, "/api/projects")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v (%s)", err, rec.Body.String())
	}
	if len(got) != 0 {
		t.Fatalf("projects = %v, want empty array", got)
	}
}

func TestListProjects_Populated(t *testing.T) {
	t.Parallel()
	mine := mustProject(t, "web-api", "setagaya", "1")
	other := mustProject(t, "other", "someone-else", "2")
	router, _ := newTestRouter(t, mine, other)

	rec := do(t, router, http.MethodGet, "/api/projects")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Owner string `json:"owner"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Default owner is "setagaya" so only the matching project is returned.
	if len(got) != 1 || got[0].Name != "web-api" || got[0].Owner != "setagaya" {
		t.Fatalf("projects = %+v, want only web-api owned by setagaya", got)
	}
}

func TestGetProject_FoundAndNotFound(t *testing.T) {
	t.Parallel()
	mine := mustProject(t, "web-api", "setagaya", "1")
	router, ids := newTestRouter(t, mine)

	rec := do(t, router, http.MethodGet, "/api/projects/"+strconv.FormatInt(ids[0], 10))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	miss := do(t, router, http.MethodGet, "/api/projects/424242")
	if miss.Code != http.StatusNotFound {
		t.Fatalf("missing project status = %d, want 404", miss.Code)
	}

	bad := do(t, router, http.MethodGet, "/api/projects/not-an-int")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", bad.Code)
	}
}

func mustProject(t *testing.T, name, owner, sid string) project.Project {
	t.Helper()
	p, err := project.New(name, owner, sid)
	if err != nil {
		t.Fatalf("build project: %v", err)
	}
	return p
}
