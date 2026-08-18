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
	"time"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/account"
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

// doRaw sends a request with a literal form-encoded body -- for bodies (like a
// deliberately malformed form) that url.Values cannot express.
func doRaw(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// patchForm is putForm's PATCH sibling (tenant status uses PATCH).
func patchForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

// A scenario already pinned native by an uploaded script has nothing that
// ever reads stored requests -- accepting the upload would be a silent no-op,
// so it is rejected as a conflict instead.
func TestScenarioRequests_RejectsNativeScenarioIs409(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"native"}, "project_id": {itoa(projectID)}}))
	putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", "scenario.jmx", "<jmx/>")

	rec := putMultipart(t, h, "/api/scenarios/"+itoa(scenarioID)+"/requests", "requests.yml", "requests:\n  - url: /checkout\n")
	if rec.Code != http.StatusConflict {
		t.Fatalf("upload requests for native scenario = %d, want 409 (%s)", rec.Code, rec.Body.String())
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

// Every form-consuming endpoint answers 400 to an unparseable body rather than
// panicking or 500ing -- one request per handler, across the routers that wire
// each dependency.
func TestHandlers_MalformedForms_400(t *testing.T) {
	t.Parallel()
	campaigns, _ := newCampaignRouter(t)
	calib, _, _ := newCalibrationRouter(t)
	admin, _ := newAdminRouter(t)
	cluster := newClusterRouter(&stubClusterService{})
	cases := []struct {
		h            http.Handler
		method, path string
	}{
		{newFullRouter(t), http.MethodPost, "/api/projects"},
		{newFullRouter(t), http.MethodPost, "/api/scenarios"},
		{newFullRouter(t), http.MethodPost, "/api/executions"},
		{newTenantRouter(t), http.MethodPost, "/api/tenants"},
		{newTenantRouter(t), http.MethodPatch, "/api/tenants/1"},
		{newTenantRouter(t), http.MethodPut, "/api/tenants/1/quota"},
		{newTenantRouter(t), http.MethodPost, "/api/tenants/1/roles"},
		{newTenantRouter(t), http.MethodPost, "/api/roles"},
		{campaigns, http.MethodPost, "/api/tenants/1/campaigns"},
		{calib, http.MethodPost, "/api/calibrations"},
		{admin, http.MethodPost, "/api/admin/abort"},
		{cluster, http.MethodPost, "/api/clusters"},
		{cluster, http.MethodPut, "/api/clusters/prod-eu"},
	}
	for _, tc := range cases {
		if rec := doRaw(t, tc.h, tc.method, tc.path, "%zz=1"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s (malformed form) = %d, want 400 (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// Tenant admin routes: invalid path ids, invalid enum/numeric form values, a
// quota read for a missing tenant, and the role-assignment validation errors --
// plus a global assign/revoke round-trip.
func TestTenantHandlers_Validation(t *testing.T) {
	t.Parallel()
	h := newTenantRouter(t)

	if rec := postForm(t, h, "/api/tenants", url.Values{"name": {""}, "display_name": {"X"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("create tenant (empty name) = %d, want 400", rec.Code)
	}

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/tenants/x"},
		{http.MethodPatch, "/api/tenants/x"},
		{http.MethodPut, "/api/tenants/x/quota"},
		{http.MethodGet, "/api/tenants/x/quota"},
		{http.MethodPost, "/api/tenants/x/roles"},
		{http.MethodDelete, "/api/tenants/x/roles"},
	} {
		if rec := do(t, h, tc.method, tc.path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tc.method, tc.path, rec.Code)
		}
	}

	if rec := patchForm(t, h, "/api/tenants/1", url.Values{"status": {"frozen"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("set status (invalid) = %d, want 400", rec.Code)
	}
	if rec := putForm(t, h, "/api/tenants/1/quota", url.Values{"ceiling": {"many"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("set quota (non-numeric ceiling) = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/tenants/999999/quota"); rec.Code != http.StatusNotFound {
		t.Errorf("get quota (missing tenant) = %d, want 404", rec.Code)
	}

	if rec := postForm(t, h, "/api/roles", url.Values{"role": {"tenant_admin"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("assign role (no subject) = %d, want 400", rec.Code)
	}
	if rec := postForm(t, h, "/api/roles", url.Values{"subject": {"bob"}, "role": {"wizard"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("assign role (unknown) = %d, want 400", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/roles", url.Values{"subject": {""}, "role": {""}}); rec.Code != http.StatusBadRequest {
		t.Errorf("revoke role (missing params) = %d, want 400", rec.Code)
	}

	// A global role round-trips through the global endpoints (no tenant id).
	if rec := postForm(t, h, "/api/roles", url.Values{"subject": {"bob"}, "role": {"service_provider_admin"}}); rec.Code != http.StatusCreated {
		t.Fatalf("assign global role = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := deleteWithQuery(t, h, "/api/roles", url.Values{"subject": {"bob"}, "role": {"service_provider_admin"}}); rec.Code != http.StatusOK {
		t.Fatalf("revoke global role = %d (%s)", rec.Code, rec.Body.String())
	}
}

// Creating a campaign validates its window and service pairs before anything
// else runs; a mismatched or non-numeric pair, or an unparsable window bound,
// is a 400 naming the field.
func TestCampaignHandlers_CreateValidation(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	start := "2027-01-01T09:00:00Z"
	end := "2027-01-01T11:00:00Z"

	cases := []struct {
		name string
		form url.Values
	}{
		{"bad window_start", url.Values{"name": {"S"}, "window_start": {"not-a-time"}, "window_end": {end}}},
		{"bad window_end", url.Values{"name": {"S"}, "window_start": {start}, "window_end": {"not-a-time"}}},
		{"bad service_project_id", url.Values{"name": {"S"}, "window_start": {start}, "window_end": {end}, "service_project_id": {"abc"}, "service_execution_id": {"1"}}},
		{"bad service_execution_id", url.Values{"name": {"S"}, "window_start": {start}, "window_end": {end}, "service_project_id": {"1"}, "service_execution_id": {"abc"}}},
	}
	for _, tc := range cases {
		if rec := postForm(t, h, "/api/tenants/1/campaigns", tc.form); rec.Code != http.StatusBadRequest {
			t.Errorf("create campaign (%s) = %d, want 400 (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}
	if rec := do(t, h, http.MethodGet, "/api/campaigns/x/comparison"); rec.Code != http.StatusBadRequest {
		t.Errorf("comparison (invalid id) = %d, want 400", rec.Code)
	}
}

// Campaigns are an optional dependency: with them unwired, create, get, and
// comparison are all 404s.
func TestCampaignHandlers_NotConfiguredRoutes(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/tenants/1/campaigns"},
		{http.MethodGet, "/api/campaigns/1"},
		{http.MethodGet, "/api/campaigns/1/comparison"},
	} {
		if rec := do(t, h, tc.method, tc.path); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s (not configured) = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

// Reservation-window parsing: a non-numeric tenant id or an unparsable `to`
// bound is a 400 before the (here unwired) reservation backend is touched.
func TestTenantReservations_InvalidParams_400(t *testing.T) {
	t.Parallel()
	h := newTenantRouter(t)
	if rec := do(t, h, http.MethodGet, "/api/tenants/x/reservations"); rec.Code != http.StatusBadRequest {
		t.Errorf("reservations (invalid id) = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/tenants/1/reservations?to=not-a-time"); rec.Code != http.StatusBadRequest {
		t.Errorf("reservations (bad to) = %d, want 400", rec.Code)
	}
}

// The global role endpoints share the tenant gate: without tenants configured
// they are 404s like every other optional dependency.
func TestGlobalRoles_NotConfigured_404(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{})
	if rec := do(t, h, http.MethodPost, "/api/roles"); rec.Code != http.StatusNotFound {
		t.Errorf("assign global role (not configured) = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/roles"); rec.Code != http.StatusNotFound {
		t.Errorf("revoke global role (not configured) = %d, want 404", rec.Code)
	}
}

// A caller with no grant in the campaign's tenant, and none on any
// participating project, may not even read it: list and get need a tenant
// grant, verdict and comparison need at least one participating project.
func TestRBAC_CampaignReadRequiresAGrant(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")
	projectID := createProjectInTenantReturningID(t, f, "acme-web", "team-a", acme)
	executionID := decodeID(t, f.req(t, http.MethodPost, "/api/executions", "admin-tok",
		url.Values{"name": {"readiness"}, "project_id": {itoa(projectID)}}))

	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	form := url.Values{
		"name": {"Supersale"}, "window_start": {start}, "window_end": {end},
		"service_project_id":   {itoa(projectID)},
		"service_execution_id": {itoa(executionID)},
	}
	created := f.req(t, http.MethodPost, "/api/tenants/"+itoa(acme)+"/campaigns", "admin-tok", form)
	if created.Code != http.StatusCreated {
		t.Fatalf("create campaign = %d (%s)", created.Code, created.Body.String())
	}
	campaignID := decodeID(t, created)

	f.prov.Register("eve-tok", account.Account{Subject: "eve"})
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/tenants/" + itoa(acme) + "/campaigns"},
		{http.MethodGet, "/api/campaigns/" + itoa(campaignID)},
		{http.MethodGet, "/api/campaigns/" + itoa(campaignID) + "/verdict"},
		{http.MethodGet, "/api/campaigns/" + itoa(campaignID) + "/comparison"},
	} {
		if rec := f.req(t, tc.method, tc.path, "eve-tok", nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s (no grant) = %d, want 403 (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// The numeric calibration bounds are validated field by field -- a non-numeric
// value is a 400 naming the field, not a silent zero.
func TestCalibrationHandlers_InvalidBounds_400(t *testing.T) {
	t.Parallel()
	h, _, _ := newCalibrationRouter(t)
	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))

	base := url.Values{
		"name": {"cal"}, "project_id": {itoa(projectID)}, "engine": {"jmeter"},
		"criterion": {"qps"}, "cpu": {"1"}, "memory": {"1Gi"},
	}
	for _, field := range []string{"seed_qps", "max_qps", "max_steps", "hold_seconds"} {
		form := url.Values{}
		for k, v := range base {
			form[k] = v
		}
		form.Set(field, "not-a-number")
		if rec := postForm(t, h, "/api/calibrations", form); rec.Code != http.StatusBadRequest {
			t.Errorf("create calibration (%s=abc) = %d, want 400 (%s)", field, rec.Code, rec.Body.String())
		}
	}
}

// The capacity-profile endpoints authorize through the scenario's project: a
// scenario the caller does not own is a 403 before any profile lookup.
func TestCalibrationCapacityRoutes_ForeignScenario_403(t *testing.T) {
	t.Parallel()
	h, store, _ := newCalibrationRouter(t)
	foreign, _ := store.CreateProject(context.Background(), mustProject(t, "secret", "other-team", ""))
	pl, _ := scenario.New("smoke", foreign)
	scenarioID, _ := store.CreateScenario(context.Background(), pl)

	profile := "/api/scenarios/" + itoa(scenarioID) + "/capacity-profile?engine=jmeter&cpu=1&memory=1Gi"
	if rec := do(t, h, http.MethodGet, profile); rec.Code != http.StatusForbidden {
		t.Errorf("capacity profile (foreign scenario) = %d, want 403", rec.Code)
	}
	fanout := profile + "&target_qps=100"
	if rec := do(t, h, http.MethodGet, fanout); rec.Code != http.StatusForbidden {
		t.Errorf("capacity fanout (foreign scenario) = %d, want 403", rec.Code)
	}
}

// Uploading to file endpoints: authorization is checked before the multipart
// body is parsed (a foreign id is 403 even for a body-less request), and a
// body that is not a multipart upload with a "file" field is a 400.
func TestFileUploadRoutes_AuthorizationAndBodies(t *testing.T) {
	t.Parallel()
	h, store, _ := routerWithStore(t)
	ctx := context.Background()

	foreignProject, _ := store.CreateProject(ctx, mustProject(t, "secret", "other-team", ""))
	foreignScenario, _ := scenario.New("p", foreignProject)
	foreignScenarioID, _ := store.CreateScenario(ctx, foreignScenario)
	foreignColl, _ := execution.New("c", foreignProject)
	foreignCollID, _ := store.CreateExecution(ctx, foreignColl)

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}}))
	collID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))

	// Foreign ids are rejected before any body parsing.
	forbidden := []struct{ method, path string }{
		{http.MethodPut, "/api/executions/" + itoa(foreignCollID) + "/files"},
		{http.MethodDelete, "/api/executions/" + itoa(foreignCollID) + "/files?filename=x.csv"},
		{http.MethodPut, "/api/executions/" + itoa(foreignCollID) + "/config"},
		{http.MethodPut, "/api/scenarios/" + itoa(foreignScenarioID) + "/files"},
		{http.MethodDelete, "/api/scenarios/" + itoa(foreignScenarioID) + "/files?filename=x.csv"},
		{http.MethodPut, "/api/scenarios/" + itoa(foreignScenarioID) + "/requests"},
	}
	for _, tc := range forbidden {
		if rec := do(t, h, tc.method, tc.path); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s (foreign) = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}

	// Own ids, but no multipart file in the body.
	for _, tc := range []struct{ method, path string }{
		{http.MethodPut, "/api/executions/" + itoa(collID) + "/files"},
		{http.MethodPut, "/api/executions/" + itoa(collID) + "/config"},
		{http.MethodPut, "/api/scenarios/" + itoa(scenarioID) + "/files"},
		{http.MethodPut, "/api/scenarios/" + itoa(scenarioID) + "/requests"},
	} {
		if rec := doRaw(t, h, tc.method, tc.path, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s (no file) = %d, want 400", tc.method, tc.path, rec.Code)
		}
	}

	// Deleting a file that was never uploaded is a 404, not a silent success.
	if rec := deleteWithQuery(t, h, "/api/executions/"+itoa(collID)+"/files", url.Values{"filename": {"missing.csv"}}); rec.Code != http.StatusNotFound {
		t.Errorf("delete execution file (missing) = %d, want 404", rec.Code)
	}
	if rec := deleteWithQuery(t, h, "/api/scenarios/"+itoa(scenarioID)+"/files", url.Values{"filename": {"missing.csv"}}); rec.Code != http.StatusNotFound {
		t.Errorf("delete scenario file (missing) = %d, want 404", rec.Code)
	}

	// A missing execution's config read is a 404.
	if rec := do(t, h, http.MethodGet, "/api/executions/999999/config"); rec.Code != http.StatusNotFound {
		t.Errorf("get config (missing execution) = %d, want 404", rec.Code)
	}
}

// The JMX import endpoint validates its upload, its project id, and project
// ownership in that order: each has a dedicated failure.
func TestImportJMX_Validation(t *testing.T) {
	t.Parallel()
	h, store, _ := routerWithStore(t)
	foreign, _ := store.CreateProject(context.Background(), mustProject(t, "secret", "other-team", ""))

	// No file in the body at all.
	if rec := doRaw(t, h, http.MethodPost, "/api/scenarios/import", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("import (no file) = %d, want 400", rec.Code)
	}
	// A file, but a project_id that is not a number.
	if rec := postMultipart(t, h, "/api/scenarios/import", "x.jmx", "<jmx/>", map[string]string{"project_id": "abc"}); rec.Code != http.StatusBadRequest {
		t.Errorf("import (bad project_id) = %d, want 400", rec.Code)
	}
	// A valid project the caller does not own.
	if rec := postMultipart(t, h, "/api/scenarios/import", "x.jmx", "<jmx/>", map[string]string{"project_id": itoa(foreign)}); rec.Code != http.StatusForbidden {
		t.Errorf("import (foreign project) = %d, want 403", rec.Code)
	}
}

// newLifecycleBareRouter wires the lifecycle service but leaves
// TriggerReadyPoll/TriggerReadyTimeout unset, so trigger requests run with the
// default bounded-wait settings.
func newLifecycleBareRouter(t *testing.T) http.Handler {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	sched := fake.NewScheduler()
	return httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 100),
		Lifecycle:     lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("img")),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})
}

// Stream, status, and engines reads validate the id / surface missing-execution
// errors through the usual error mapping.
func TestExecutionReadEndpoints_Validation(t *testing.T) {
	t.Parallel()
	h := newLifecycleBareRouter(t)
	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	collID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))

	if rec := do(t, h, http.MethodGet, "/api/executions/x/stream"); rec.Code != http.StatusBadRequest {
		t.Errorf("stream (invalid id) = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/executions/999999/status"); rec.Code != http.StatusNotFound {
		t.Errorf("status (missing execution) = %d, want 404", rec.Code)
	}
	// Trigger with the default (unset) readiness settings: an execution with no
	// load profile fails immediately with a non-readiness error, proving the
	// default poll/timeout wiring returns without looping.
	if rec := do(t, h, http.MethodPost, "/api/executions/"+itoa(collID)+"/trigger"); rec.Code == http.StatusOK {
		t.Errorf("trigger (no load profile) = %d, want an error status", rec.Code)
	}
}
