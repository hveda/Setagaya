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
