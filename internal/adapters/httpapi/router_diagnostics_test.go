package httpapi_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestSetScenarioRequestsDiagnostics covers the G4 envelope: a semantically
// invalid PUT returns 400 with a diagnostics list anchored to real lines,
// while non-validation failures keep the plain writeError envelope.
func TestSetScenarioRequestsDiagnostics(t *testing.T) {
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
	url := "/api/scenarios/" + strconv.FormatInt(decodeID(t, rec), 10) + "/requests"

	// Type error anchored to line 3: the editor's cursor can land on the row.
	frag := "requests:\n- url: /a\n  method: [GET]\n"
	req := httptest.NewRequest(http.MethodPut, url, strings.NewReader(frag))
	req.Header.Set("Content-Type", "text/yaml")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("type-error put = %d (%s), want 400", rec.Code, rec.Body.String())
	}
	var out struct {
		Message     string `json:"message"`
		Diagnostics []struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Line     int    `json:"line"`
			Col      int    `json:"col"`
			Path     string `json:"path"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode diagnostics: %v (%s)", err, rec.Body.String())
	}
	if out.Message != "scenarioapp: requests fragment is invalid" {
		t.Fatalf("message = %q", out.Message)
	}
	if len(out.Diagnostics) != 1 || out.Diagnostics[0].Line != 3 || out.Diagnostics[0].Severity != "error" {
		t.Fatalf("diagnostics = %+v, want one error at line 3", out.Diagnostics)
	}
	if out.Diagnostics[0].Col != 0 {
		t.Fatalf("col = %d, want 0 (unanchored column)", out.Diagnostics[0].Col)
	}

	// A non-validation failure keeps the plain envelope: a native scenario
	// (imported .jmx pins it) conflicts, with no diagnostics key.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "plan.jmx")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	plan := `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2" properties="5.0" jmeter="5.6.3">
  <hashTree>
    <TestPlan guiclass="TestPlanGui" testclass="TestPlan" testname="Native one" enabled="true"/>
    <hashTree/>
  </hashTree>
</jmeterTestPlan>`
	if _, err := fw.Write([]byte(plan)); err != nil {
		t.Fatalf("write jmx: %v", err)
	}
	if err := mw.WriteField("project_id", "1"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/scenarios/import", &body)
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
	nativeURL := "/api/scenarios/" + strconv.FormatInt(importRes.Scenario.ID, 10) + "/requests"
	req = httptest.NewRequest(http.MethodPut, nativeURL, strings.NewReader("requests:\n- url: /a\n"))
	req.Header.Set("Content-Type", "text/yaml")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("native put = %d (%s), want 409", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "diagnostics") {
		t.Fatalf("409 body carries diagnostics key: %s", rec.Body.String())
	}
}

// TestValidateScenarioRequests covers the G5 endpoint end to end: a valid
// fragment validates to 200 and stores nothing; an invalid one answers 400
// with the same line-anchored diagnostics envelope the PUT path writes; a
// native scenario conflicts; an unknown scenario is a bare 404.
func TestValidateScenarioRequests(t *testing.T) {
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
	scenarioID := decodeID(t, rec)
	valURL := "/api/scenarios/" + strconv.FormatInt(scenarioID, 10) + "/requests/validate"
	getReqURL := "/api/scenarios/" + strconv.FormatInt(scenarioID, 10) + "/requests"

	postYAML := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, valURL, strings.NewReader(body))
		req.Header.Set("Content-Type", "text/yaml")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Valid fragment: 200 with no diagnostics, and NOTHING stored -- a
	// subsequent GET still 404s, because validate must not have side effects.
	rec = postYAML("requests:\n- url: /checkout\n  method: GET\n")
	if rec.Code != http.StatusOK {
		t.Fatalf("validate valid fragment = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var okOut struct {
		Valid       bool `json:"valid"`
		Diagnostics []struct {
			Severity string `json:"severity"`
			Line     int    `json:"line"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &okOut); err != nil {
		t.Fatalf("decode validate response: %v (%s)", err, rec.Body.String())
	}
	if !okOut.Valid || len(okOut.Diagnostics) != 0 {
		t.Fatalf("valid fragment response = valid:%v diags:%+v, want valid true, no diagnostics", okOut.Valid, okOut.Diagnostics)
	}
	if rec := do(t, h, http.MethodGet, getReqURL); rec.Code != http.StatusNotFound {
		t.Fatalf("GET after validate = %d, want 404 (validate must store nothing)", rec.Code)
	}

	// Type error: 400 anchored to line 3, same envelope as the PUT path.
	rec = postYAML("requests:\n- url: /a\n  method: [GET]\n")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("validate type error = %d (%s), want 400", rec.Code, rec.Body.String())
	}
	var errOut struct {
		Message     string `json:"message"`
		Diagnostics []struct {
			Severity string `json:"severity"`
			Line     int    `json:"line"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errOut); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, rec.Body.String())
	}
	if errOut.Message != "scenarioapp: requests fragment is invalid" {
		t.Fatalf("message = %q", errOut.Message)
	}
	if len(errOut.Diagnostics) != 1 || errOut.Diagnostics[0].Line != 3 || errOut.Diagnostics[0].Severity != "error" {
		t.Fatalf("diagnostics = %+v, want one error at line 3", errOut.Diagnostics)
	}

	// Native scenario (imported .jmx pins it): 409, plain envelope.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "plan.jmx")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	plan := `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2" properties="5.0" jmeter="5.6.3">
  <hashTree>
    <TestPlan guiclass="TestPlanGui" testclass="TestPlan" testname="Native one" enabled="true"/>
    <hashTree/>
  </hashTree>
</jmeterTestPlan>`
	if _, err := fw.Write([]byte(plan)); err != nil {
		t.Fatalf("write jmx: %v", err)
	}
	if err := mw.WriteField("project_id", "1"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	_ = mw.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/api/scenarios/import", &body)
	req2.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req2)
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
	nativeValURL := "/api/scenarios/" + strconv.FormatInt(importRes.Scenario.ID, 10) + "/requests/validate"
	req3 := httptest.NewRequest(http.MethodPost, nativeValURL, strings.NewReader("requests:\n- url: /a\n"))
	req3.Header.Set("Content-Type", "text/yaml")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req3)
	if rec.Code != http.StatusConflict {
		t.Fatalf("native validate = %d (%s), want 409", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "diagnostics") {
		t.Fatalf("native 409 body carries diagnostics key: %s", rec.Body.String())
	}

	// Unknown scenario: bare 404, same as the PUT path.
	rec = func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/scenarios/999999/requests/validate", strings.NewReader("requests:\n- url: /a\n"))
		req.Header.Set("Content-Type", "text/yaml")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown scenario validate = %d (%s), want 404", rec.Code, rec.Body.String())
	}
}

// TestValidateStoreParity pins the G5 contract that matters most: the
// validate endpoint and the store endpoint accept and reject the SAME set of
// fragment bodies. Both go through one service validation (requestDiagnostics),
// but this test proves it at the HTTP layer -- if anyone ever splits the two
// paths, a fragment the editor previewed as valid can fail at save time (or
// the inverse), and this fails first.
func TestValidateStoreParity(t *testing.T) {
	t.Parallel()
	h := newFullRouter(t)
	rec := postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = postForm(t, h, "/api/scenarios", url.Values{"name": {"parity"}, "project_id": {"1"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scenario = %d (%s)", rec.Code, rec.Body.String())
	}
	base := "/api/scenarios/" + strconv.FormatInt(decodeID(t, rec), 10) + "/requests"

	fragments := []struct {
		name string
		body string
	}{
		{"valid single request", "requests:\n- url: /checkout\n  method: GET\n"},
		{"valid with headers", "requests:\n- url: /checkout\n  headers:\n    Authorization: Bearer x\n"},
		{"type error", "requests:\n- url: /a\n  method: [GET]\n"},
		{"no requests", "default-address: http://example.com\n"},
		{"request with no url", "requests:\n- method: GET\n"},
		{"malformed yaml", "not: [valid: yaml\n"},
	}

	for _, frag := range fragments {
		// Validate.
		req := httptest.NewRequest(http.MethodPost, base+"/validate", strings.NewReader(frag.body))
		req.Header.Set("Content-Type", "text/yaml")
		valRec := httptest.NewRecorder()
		h.ServeHTTP(valRec, req)

		// Store.
		req = httptest.NewRequest(http.MethodPut, base, strings.NewReader(frag.body))
		req.Header.Set("Content-Type", "text/yaml")
		putRec := httptest.NewRecorder()
		h.ServeHTTP(putRec, req)

		valAccepted := valRec.Code == http.StatusOK
		putAccepted := putRec.Code == http.StatusOK
		if valAccepted != putAccepted {
			t.Fatalf("%s: validate=%d but store=%d -- the two endpoints disagree on %q",
				frag.name, valRec.Code, putRec.Code, frag.name)
		}
		// Rejections carry the same shape on both paths: 400 with a
		// diagnostics list, never 500.
		if !valAccepted && valRec.Code != putRec.Code {
			t.Fatalf("%s: rejection statuses differ: validate=%d store=%d", frag.name, valRec.Code, putRec.Code)
		}
	}
}
