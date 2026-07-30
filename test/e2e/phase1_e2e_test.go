//go:build e2e

package e2e_test

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
	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/internal/adapters/storage/local"
	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

// TestPhase1_FullFlowEndToEnd drives the whole Phase 1 surface over real HTTP,
// backed by a real MySQL container and a filesystem object store.
func TestPhase1_FullFlowEndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Plans:         scenarioapp.NewService(repo, store),
		Collections:   executionapp.NewService(repo, store, 500),
		Store:         store,
		DefaultOwners: []string{"setagaya"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}})
	scenarioID := postForm(t, client, srv.URL+"/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})

	// Upload a JMX file, then download it back through the file endpoint.
	putMultipart(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/files", "plan.jmx", "<jmx>hello</jmx>")
	body := getBody(t, client, srv.URL+"/api/files/scenario/"+itoa(scenarioID)+"/plan.jmx", http.StatusOK)
	if body != "<jmx>hello</jmx>" {
		t.Fatalf("downloaded artifact = %q", body)
	}

	collID := postForm(t, client, srv.URL+"/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	cfg := fmt.Sprintf("multi-test:\n  collectionid: %d\n  csv_split: true\n  tests:\n    - testid: %d\n      concurrency: 10\n      rampup: 1\n      engines: 3\n      duration: 60\n", collID, scenarioID)
	putMultipart(t, client, srv.URL+"/api/executions/"+itoa(collID)+"/config", "config.yaml", cfg)

	// The persisted config round-trips through GET.
	got := getBody(t, client, srv.URL+"/api/executions/"+itoa(collID)+"/config", http.StatusOK)
	if !strings.Contains(got, "\"engines\":3") {
		t.Fatalf("config get missing engines: %s", got)
	}
}

func postForm(t *testing.T, client *http.Client, urlStr string, form url.Values) int64 {
	t.Helper()
	resp, err := client.PostForm(urlStr, form)
	if err != nil {
		t.Fatalf("POST %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST %s status = %d, want 201", urlStr, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	decode(t, resp, &out)
	return out.ID
}

func putMultipart(t *testing.T, client *http.Client, urlStr, filename, content string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPut, urlStr, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT %s status = %d, want 200", urlStr, resp.StatusCode)
	}
}

func getBody(t *testing.T, client *http.Client, urlStr string, wantStatus int) string {
	t.Helper()
	resp, err := client.Get(urlStr)
	if err != nil {
		t.Fatalf("GET %s: %v", urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", urlStr, resp.StatusCode, wantStatus)
	}
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if readErr != nil {
			break
		}
	}
	return sb.String()
}

func decode(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
