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

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/ports/fake"
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

// TestListExecutions covers GET /api/executions: only the caller's projects'
// executions come back, newest first, and an operator with no projects sees
// an empty list rather than every execution.
func TestListExecutions(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// One visible project with three executions; ordering is what the list
	// contract pins here (scoping itself is pinned at the repository
	// contract, which runs against the fake and real MySQL alike).
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", rec.Code, rec.Body.String())
	}

	mkExec := func(name string) int64 {
		rec := postForm(t, h, "/api/executions", url.Values{"name": {name}, "project_id": {"1"}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create execution %q = %d (%s)", name, rec.Code, rec.Body.String())
		}
		return decodeID(t, rec)
	}
	first := mkExec("first")
	mkExec("middle")
	last := mkExec("last")

	rec = do(t, h, http.MethodGet, "/api/executions")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		ProjectID int64  `json:"project_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v (%s)", err, rec.Body.String())
	}
	if len(got) != 3 {
		t.Fatalf("executions = %+v, want all three in newest-first order", got)
	}
	if got[0].ID != last || got[2].ID != first {
		t.Fatalf("order = [%d %d %d], want newest first (last, middle, first)", got[0].ID, got[1].ID, got[2].ID)
	}

	// Empty ownership: no visible projects, empty list -- never a 500 or a
	// full dump. (Router built with a different owner set.)
	emptyRouter := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(fake.NewStore()),
		Executions:    executionapp.NewService(fake.NewStore(), fake.NewObjectStore(), 100),
		DefaultOwners: []string{"nobody"},
	})
	rec = do(t, emptyRouter, http.MethodGet, "/api/executions")
	if rec.Code != http.StatusOK {
		t.Fatalf("empty status = %d", rec.Code)
	}
	var none []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &none); err != nil || len(none) != 0 {
		t.Fatalf("empty list = %v err=%v, want []", none, err)
	}
}

// TestGetScenarioRequests covers GET /api/scenarios/{id}/requests: the
// fragment comes back byte-for-byte as text/yaml, 404 when nothing was ever
// uploaded, and 409 for a native scenario.
func TestGetScenarioRequests(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	// Project + portable scenario.
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = postForm(t, h, "/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {"1"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scenario = %d (%s)", rec.Code, rec.Body.String())
	}
	scenarioID := decodeID(t, rec)

	// Nothing uploaded yet -> 404.
	if rec := do(t, h, http.MethodGet, "/api/scenarios/"+strconv.FormatInt(scenarioID, 10)+"/requests"); rec.Code != http.StatusNotFound {
		t.Fatalf("no-upload status = %d (%s), want 404", rec.Code, rec.Body.String())
	}

	// Upload a fragment with a comment, unusual key order, and an unmodelled
	// key -- exactly what a byte-preserving round trip must keep.
	fragment := "# hand-written header comment\n" +
		"think-time: 2s  # unmodelled key\n" +
		"default-address: http://target.local\n" +
		"requests:\n" +
		"- url: /cart\n" +
		"  method: POST\n"
	if rec := putMultipart(t, h, "/api/scenarios/"+strconv.FormatInt(scenarioID, 10)+"/requests", "requests.yaml", fragment); rec.Code != http.StatusOK {
		t.Fatalf("upload = %d (%s)", rec.Code, rec.Body.String())
	}

	// Fetch: byte-identical, text/yaml content type.
	rec = do(t, h, http.MethodGet, "/api/scenarios/"+strconv.FormatInt(scenarioID, 10)+"/requests")
	if rec.Code != http.StatusOK {
		t.Fatalf("fetch = %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/yaml") {
		t.Fatalf("Content-Type = %q, want text/yaml", ct)
	}
	if rec.Body.String() != fragment {
		t.Fatalf("round trip not byte-identical:\n got: %q\nwant: %q", rec.Body.String(), fragment)
	}

	// Native scenario -> 409, same stance as the PUT (import is multipart).
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "plan.jmx")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	plan := `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2" properties="5.0" jmeter="5.6.3">
  <hashTree>
    <TestPlan guiclass="TestPlanGui" testclass="TestPlan" testname="Checkout journey" enabled="true"/>
    <hashTree/>
  </hashTree>
</jmeterTestPlan>`
	if _, err := fw.Write([]byte(plan)); err != nil {
		t.Fatalf("write jmx: %v", err)
	}
	if err := mw.WriteField("project_id", "1"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := mw.WriteField("name", "native-one"); err != nil {
		t.Fatalf("write name: %v", err)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/scenarios/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import = %d (%s)", rec.Code, rec.Body.String())
	}
	var importRes struct {
		Scenario struct {
			ID int64 `json:"id"`
		} `json:"scenario"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &importRes); err != nil {
		t.Fatalf("decode import response: %v (%s)", err, rec.Body.String())
	}
	nativeID := importRes.Scenario.ID
	if rec := do(t, h, http.MethodGet, "/api/scenarios/"+strconv.FormatInt(nativeID, 10)+"/requests"); rec.Code != http.StatusConflict {
		t.Fatalf("native fetch status = %d (%s), want 409", rec.Code, rec.Body.String())
	}

	// Unknown scenario -> 404.
	if rec := do(t, h, http.MethodGet, "/api/scenarios/424242/requests"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown scenario status = %d, want 404", rec.Code)
	}
}

// TestSetScenarioRequestsRawYAML covers the raw text/yaml body on the PUT:
// the bytes are stored verbatim, a G2 -> G3 -> G2 round trip returns them
// byte-identical (comments, key order, unmodelled keys), and multipart
// uploads keep working unchanged.
func TestSetScenarioRequestsRawYAML(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)

	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = postForm(t, h, "/api/scenarios", url.Values{"name": {"checkout"}, "project_id": {"1"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scenario = %d (%s)", rec.Code, rec.Body.String())
	}
	scenarioID := strconv.FormatInt(decodeID(t, rec), 10)
	url := "/api/scenarios/" + scenarioID + "/requests"

	// Raw body with every byte-preservation trap: leading comment, unusual
	// key order, inline comment, unmodelled key, trailing newline.
	fragment := "# operator notes: bump think-time after warmup\n" +
		"think-time: 1.5s\n" +
		"default-address: https://target.local\n" +
		"requests:\n" +
		"- url: /login\n" +
		"  method: POST\n" +
		"- url: /cart\n"
	req := httptest.NewRequest(http.MethodPut, url, strings.NewReader(fragment))
	req.Header.Set("Content-Type", "text/yaml; charset=utf-8")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("raw put = %d (%s)", rec.Code, rec.Body.String())
	}

	// Round trip through G2: byte-identical, including the trailing newline.
	rec = do(t, h, http.MethodGet, url)
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != fragment {
		t.Fatalf("round trip not byte-identical:\n got: %q\nwant: %q", got, fragment)
	}

	// application/x-yaml is accepted too.
	alt := "requests:\n- url: /alt\n"
	req = httptest.NewRequest(http.MethodPut, url, strings.NewReader(alt))
	req.Header.Set("Content-Type", "application/x-yaml")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("x-yaml put = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := do(t, h, http.MethodGet, url).Body.String(); got != alt {
		t.Fatalf("x-yaml round trip = %q, want %q", got, alt)
	}

	// Multipart still works unchanged (the pre-existing path).
	mp := putMultipart(t, h, url, "requests.yaml", "requests:\n- url: /multipart\n")
	if mp.Code != http.StatusOK {
		t.Fatalf("multipart put = %d (%s)", mp.Code, mp.Body.String())
	}
	want := "requests:\n- url: /multipart\n"
	if got := do(t, h, http.MethodGet, url).Body.String(); got != want {
		t.Fatalf("multipart round trip = %q, want %q", got, want)
	}

	// Semantically invalid YAML via the raw path is rejected with the same
	// error semantics as the multipart path.
	bad := "requests: []"
	req = httptest.NewRequest(http.MethodPut, url, strings.NewReader(bad))
	req.Header.Set("Content-Type", "text/yaml")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty-requests put = %d (%s), want 400", rec.Code, rec.Body.String())
	}
}
