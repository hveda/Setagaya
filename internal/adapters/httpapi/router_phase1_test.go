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

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

func newFullRouter(t *testing.T) http.Handler {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	return httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 100),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
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

func TestProjectScenarioExecutionFlow(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// Create a project.
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", rec.Code, rec.Body.String())
	}
	projectID := decodeID(t, rec)

	// Create a scenario under the project.
	rec = postForm(t, h, "/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scenario = %d (%s)", rec.Code, rec.Body.String())
	}
	scenarioID := decodeID(t, rec)

	// Upload a JMX test file to the scenario.
	rec = putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", "scenario.jmx", "<jmx/>")
	if rec.Code != http.StatusOK {
		t.Fatalf("upload scenario file = %d (%s)", rec.Code, rec.Body.String())
	}

	// Scenario detail now reports the test file.
	rec = do(t, h, http.MethodGet, "/api/scenarios/"+itoa(scenarioID))
	if rec.Code != http.StatusOK {
		t.Fatalf("get scenario = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "scenario.jmx") {
		t.Fatalf("scenario detail missing test file: %s", rec.Body.String())
	}

	// Download the uploaded artifact.
	rec = do(t, h, http.MethodGet, "/api/files/scenario/"+itoa(scenarioID)+"/scenario.jmx")
	if rec.Code != http.StatusOK || rec.Body.String() != "<jmx/>" {
		t.Fatalf("download = %d %q", rec.Code, rec.Body.String())
	}

	// Create a execution and store its execution config.
	rec = postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create execution = %d (%s)", rec.Code, rec.Body.String())
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
`, collID, scenarioID)
	rec = putMultipart(t, h, "/api/executions/"+itoa(collID)+"/config", "config.yaml", configYAML)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload config = %d (%s)", rec.Code, rec.Body.String())
	}

	// The execution now reports its execution scenario.
	rec = do(t, h, http.MethodGet, "/api/executions/"+itoa(collID))
	if rec.Code != http.StatusOK {
		t.Fatalf("get execution = %d", rec.Code)
	}
	var coll struct {
		LoadProfile []struct {
			ScenarioID int64 `json:"scenario_id"`
			Engines    int   `json:"engines"`
		} `json:"load_profile"`
		CSVSplit bool `json:"csv_split"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode execution: %v", err)
	}
	if len(coll.LoadProfile) != 1 || coll.LoadProfile[0].ScenarioID != scenarioID || coll.LoadProfile[0].Engines != 2 {
		t.Fatalf("execution plans = %+v", coll.LoadProfile)
	}
	if !coll.CSVSplit {
		t.Fatalf("csv_split not persisted")
	}

	// The scenario is now in use: deleting it is a 409.
	rec = do(t, h, http.MethodDelete, "/api/scenarios/"+itoa(scenarioID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete in-use scenario = %d, want 409", rec.Code)
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

func TestCreateScenario_InvalidName_400(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	projectID := decodeID(t, rec)

	rec = postForm(t, h, "/api/scenarios", url.Values{"name": {""}, "project_id": {itoa(projectID)}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create scenario (empty name) = %d, want 400", rec.Code)
	}
}

func TestConfigUpload_EngineLimit_400(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 1), // limit of 1 engine
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})

	pr := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	projectID := decodeID(t, pr)
	pl := postForm(t, h, "/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	scenarioID := decodeID(t, pl)
	cl := postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	collID := decodeID(t, cl)

	configYAML := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 1\n      rampup: 1\n      engines: 5\n      duration: 60\n", collID, scenarioID)
	rec := putMultipart(t, h, "/api/executions/"+itoa(collID)+"/config", "config.yaml", configYAML)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("config over engine limit = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
