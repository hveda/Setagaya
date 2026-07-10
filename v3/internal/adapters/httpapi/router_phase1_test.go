package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/v3/internal/app/collectionapp"
	"github.com/heridotlife/Setagaya/v3/internal/app/planapp"
	"github.com/heridotlife/Setagaya/v3/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/v3/internal/ports/fake"
)

func newFullRouter(t *testing.T) http.Handler {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	return httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Plans:         planapp.NewService(store, obj),
		Collections:   collectionapp.NewService(store, obj, 100),
		Store:         obj,
		DefaultOwners: []string{"setagaya"},
	})
}

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func putMultipart(t *testing.T, h http.Handler, path, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPut, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeID(t *testing.T, rec *httptest.ResponseRecorder) int64 {
	t.Helper()
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode id: %v (%s)", err, rec.Body.String())
	}
	return out.ID
}

func TestProjectPlanCollectionFlow(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// Create a project.
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", rec.Code, rec.Body.String())
	}
	projectID := decodeID(t, rec)

	// Create a plan under the project.
	rec = postForm(t, h, "/api/plans", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create plan = %d (%s)", rec.Code, rec.Body.String())
	}
	planID := decodeID(t, rec)

	// Upload a JMX test file to the plan.
	rec = putMultipart(t, h, "/api/plans/"+itoa(planID)+"/files", "plan.jmx", "<jmx/>")
	if rec.Code != http.StatusOK {
		t.Fatalf("upload plan file = %d (%s)", rec.Code, rec.Body.String())
	}

	// Plan detail now reports the test file.
	rec = do(t, h, http.MethodGet, "/api/plans/"+itoa(planID))
	if rec.Code != http.StatusOK {
		t.Fatalf("get plan = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "plan.jmx") {
		t.Fatalf("plan detail missing test file: %s", rec.Body.String())
	}

	// Download the uploaded artifact.
	rec = do(t, h, http.MethodGet, "/api/files/plan/"+itoa(planID)+"/plan.jmx")
	if rec.Code != http.StatusOK || rec.Body.String() != "<jmx/>" {
		t.Fatalf("download = %d %q", rec.Code, rec.Body.String())
	}

	// Create a collection and store its execution config.
	rec = postForm(t, h, "/api/collections", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create collection = %d (%s)", rec.Code, rec.Body.String())
	}
	collID := decodeID(t, rec)

	configYAML := fmt.Sprintf(`multi-test:
  collectionid: %d
  csv_split: true
  tests:
    - testid: %d
      concurrency: 10
      rampup: 1
      engines: 2
      duration: 60
`, collID, planID)
	rec = putMultipart(t, h, "/api/collections/"+itoa(collID)+"/config", "config.yaml", configYAML)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload config = %d (%s)", rec.Code, rec.Body.String())
	}

	// The collection now reports its execution plan.
	rec = do(t, h, http.MethodGet, "/api/collections/"+itoa(collID))
	if rec.Code != http.StatusOK {
		t.Fatalf("get collection = %d", rec.Code)
	}
	var coll struct {
		ExecutionPlans []struct {
			PlanID  int64 `json:"plan_id"`
			Engines int   `json:"engines"`
		} `json:"execution_plans"`
		CSVSplit bool `json:"csv_split"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode collection: %v", err)
	}
	if len(coll.ExecutionPlans) != 1 || coll.ExecutionPlans[0].PlanID != planID || coll.ExecutionPlans[0].Engines != 2 {
		t.Fatalf("execution plans = %+v", coll.ExecutionPlans)
	}
	if !coll.CSVSplit {
		t.Fatalf("csv_split not persisted")
	}

	// The plan is now in use: deleting it is a 409.
	rec = do(t, h, http.MethodDelete, "/api/plans/"+itoa(planID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use plan = %d, want 409", rec.Code)
	}

	// Deleting the project while it has children is a 409.
	rec = do(t, h, http.MethodDelete, "/api/projects/"+itoa(projectID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete non-empty project = %d, want 409", rec.Code)
	}
}

func TestCreateProject_ForbiddenOwner(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"someone-else"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create project (foreign owner) = %d, want 403", rec.Code)
	}
}

func TestCreatePlan_InvalidName_400(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}})
	projectID := decodeID(t, rec)

	rec = postForm(t, h, "/api/plans", url.Values{"name": {""}, "project_id": {itoa(projectID)}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create plan (empty name) = %d, want 400", rec.Code)
	}
}

func TestConfigUpload_EngineLimit_400(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Plans:         planapp.NewService(store, obj),
		Collections:   collectionapp.NewService(store, obj, 1), // limit of 1 engine
		Store:         obj,
		DefaultOwners: []string{"setagaya"},
	})

	pr := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}})
	projectID := decodeID(t, pr)
	pl := postForm(t, h, "/api/plans", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	planID := decodeID(t, pl)
	cl := postForm(t, h, "/api/collections", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	collID := decodeID(t, cl)

	configYAML := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 1\n      rampup: 1\n      engines: 5\n      duration: 60\n", collID, planID)
	rec := putMultipart(t, h, "/api/collections/"+itoa(collID)+"/config", "config.yaml", configYAML)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("config over engine limit = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
