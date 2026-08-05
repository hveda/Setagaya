package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func routerWithStore(t *testing.T) (http.Handler, *fake.Store, *fake.ObjectStore) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 100),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
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

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}}))
	collID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))

	// Scenario data file: upload, list, delete.
	putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", "users.csv", "a,b")
	if rec := do(t, h, http.MethodGet, "/api/scenarios/"+itoa(scenarioID)+"/files"); rec.Code != http.StatusOK {
		t.Fatalf("list scenario files = %d", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", url.Values{"filename": {"users.csv"}}); rec.Code != http.StatusOK {
		t.Fatalf("delete scenario file = %d (%s)", rec.Code, rec.Body.String())
	}

	// Execution data file: upload, list, delete.
	putMultipart(t, h, "/api/executions/"+itoa(collID)+"/files", "shared.csv", "x,y")
	if rec := do(t, h, http.MethodGet, "/api/executions/"+itoa(collID)+"/files"); rec.Code != http.StatusOK {
		t.Fatalf("list execution files = %d", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/executions/"+itoa(collID)+"/files", url.Values{"filename": {"shared.csv"}}); rec.Code != http.StatusOK {
		t.Fatalf("delete execution file = %d", rec.Code)
	}

	// Empty config get, then delete execution, scenario, and empty project.
	if rec := do(t, h, http.MethodGet, "/api/executions/"+itoa(collID)+"/config"); rec.Code != http.StatusOK {
		t.Fatalf("get config = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/executions/"+itoa(collID)); rec.Code != http.StatusOK {
		t.Fatalf("delete execution = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/scenarios/"+itoa(scenarioID)); rec.Code != http.StatusOK {
		t.Fatalf("delete scenario = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/projects/"+itoa(projectID)); rec.Code != http.StatusOK {
		t.Fatalf("delete empty project = %d", rec.Code)
	}
}

// A portable scenario has nothing to compile until its declarative requests
// are uploaded; this exercises the endpoint that supplies them end to end
// through the router, not just the service method underneath it.
func TestScenarioRequests_UploadHappyPath(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"portable"}, "project_id": {itoa(projectID)}}))

	rec := putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/requests", "requests.yml",
		"requests:\n  - url: /checkout\n")
	if rec.Code != http.StatusOK {
		t.Fatalf("upload requests = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestScenarioRequests_RejectsInvalidFragment(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"portable"}, "project_id": {itoa(projectID)}}))

	// Valid YAML, but no requests -- rejected as a bad upload, not silently
	// accepted and left to fail at deploy time instead.
	rec := putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/requests", "requests.yml", "default-address: http://example.com\n")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload requests with none = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestScenarioRequests_UnknownScenarioIs404(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	rec := putMultipart(t, h, "/api/scenarios/999999/requests", "requests.yml", "requests:\n  - url: /checkout\n")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("upload requests for unknown scenario = %d, want 404 (%s)", rec.Code, rec.Body.String())
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
		{http.MethodPut, "/api/scenarios/x/requests"},
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
		t.Errorf("get missing scenario = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/executions/999"); rec.Code != http.StatusNotFound {
		t.Errorf("get missing execution = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/scenarios/999"); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing scenario = %d, want 404", rec.Code)
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

	// Creating a scenario/execution under a foreign project is also forbidden.
	if rec := postForm(t, h, "/api/scenarios", url.Values{"name": {"x"}, "project_id": {itoa(projectID)}}); rec.Code != http.StatusForbidden {
		t.Errorf("create scenario under foreign project = %d, want 403", rec.Code)
	}
	if rec := postForm(t, h, "/api/executions", url.Values{"name": {"x"}, "project_id": {itoa(projectID)}}); rec.Code != http.StatusForbidden {
		t.Errorf("create execution under foreign project = %d, want 403", rec.Code)
	}
}

func TestHandlers_BadFormBodies(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// Non-numeric project_id on create scenario/execution → 400.
	if rec := postForm(t, h, "/api/scenarios", url.Values{"name": {"x"}, "project_id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create scenario bad project_id = %d, want 400", rec.Code)
	}
	if rec := postForm(t, h, "/api/executions", url.Values{"name": {"x"}, "project_id": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create execution bad project_id = %d, want 400", rec.Code)
	}

	// Invalid YAML config upload → 400.
	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	collID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))
	if rec := putMultipart(t, h, "/api/executions/"+itoa(collID)+"/config", "c.yaml", "a: b: c"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid yaml = %d, want 400", rec.Code)
	}
}

// postMultipart posts a file plus form fields, which the import endpoint needs
// (the shared putMultipart helper sends no extra fields and uses PUT).
func postMultipart(t *testing.T, h http.Handler, path, filename, content string, fields map[string]string) *httptest.ResponseRecorder {
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
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("WriteField: %v", err)
		}
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Importing a Shibuya plan must produce a runnable, JMeter-pinned scenario and
// tell the user what will differ -- the findings are the point of the endpoint,
// so they must reach the response body.
func TestImportJMX_CreatesNativeScenarioAndReportsFindings(t *testing.T) {
	t.Parallel()
	h, store, _ := routerWithStore(t)
	proj, _ := store.CreateProject(context.Background(), mustProject(t, "web", "honryu", "1"))

	plan := `<?xml version="1.0"?>
<jmeterTestPlan version="1.2">
  <hashTree>
    <TestPlan testname="Checkout"/>
    <ThreadGroup testname="Shoppers">
      <stringProp name="ThreadGroup.num_threads">200</stringProp>
    </ThreadGroup>
    <CSVDataSet testname="accounts" enabled="true">
      <stringProp name="filename">/mnt/prod/accounts.csv</stringProp>
    </CSVDataSet>
  </hashTree>
</jmeterTestPlan>`

	rec := postMultipart(t, h, "/api/scenarios/import", "checkout.jmx", plan,
		map[string]string{"project_id": itoa(proj)})
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Scenario struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"scenario"`
		Report struct {
			TestPlanName string `json:"test_plan_name"`
			Findings     []struct {
				Kind string `json:"kind"`
			} `json:"findings"`
		} `json:"report"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.Scenario.Name != "checkout" {
		t.Errorf("name = %q, want the filename without its extension", got.Scenario.Name)
	}
	if got.Report.TestPlanName != "Checkout" {
		t.Errorf("test plan name = %q", got.Report.TestPlanName)
	}
	kinds := map[string]bool{}
	for _, f := range got.Report.Findings {
		kinds[f.Kind] = true
	}
	if !kinds["load-overridden"] || !kinds["unreachable-path"] {
		t.Errorf("findings = %+v, want both the overridden load and the unreachable path", got.Report.Findings)
	}

	// The plan itself is stored as the scenario's test file, so it can run.
	files := do(t, h, http.MethodGet, "/api/scenarios/"+itoa(got.Scenario.ID)+"/files")
	if !strings.Contains(files.Body.String(), "checkout.jmx") {
		t.Errorf("scenario files = %s, want the imported plan", files.Body.String())
	}
}

// A file that parses but cannot run must be refused rather than becoming a
// scenario that fails only when a pod starts.
func TestImportJMX_RefusesUnrunnablePlan(t *testing.T) {
	t.Parallel()
	h, store, _ := routerWithStore(t)
	proj, _ := store.CreateProject(context.Background(), mustProject(t, "web", "honryu", "1"))

	workbench := `<?xml version="1.0"?><jmeterTestPlan><hashTree><WorkBench testname="WorkBench"/></hashTree></jmeterTestPlan>`
	rec := postMultipart(t, h, "/api/scenarios/import", "wb.jmx", workbench,
		map[string]string{"project_id": itoa(proj)})
	if rec.Code == http.StatusCreated {
		t.Fatalf("import of a plan with no TestPlan succeeded: %s", rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "testplan") {
		t.Errorf("error does not state the reason: %s", rec.Body.String())
	}
}
