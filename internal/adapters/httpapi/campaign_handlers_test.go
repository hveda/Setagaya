package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newCampaignRouter(t *testing.T) (http.Handler, *fake.Store) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Executions:    executionapp.NewService(store, obj, 100),
		Campaigns:     campaignapp.NewService(store, fake.NewScheduler()),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})
	return h, store
}

// seedProjectAndExecution creates a project and execution under it via the
// HTTP API, and returns both ids.
func seedProjectAndExecution(t *testing.T, h http.Handler, name string) (projectID, executionID int64) {
	t.Helper()
	projectID = decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {name}, "owner": {"honryu"}}))
	executionID = decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"readiness"}, "project_id": {itoa(projectID)}}))
	return projectID, executionID
}

type campaignRow struct {
	ID       int64 `json:"id"`
	TenantID int64 `json:"tenant_id"`
	Services []struct {
		ProjectID   int64 `json:"project_id"`
		ExecutionID int64 `json:"execution_id"`
	} `json:"services"`
	Active bool `json:"active"`
}

func TestCreateCampaign_AdmitsAndReturnsCreated(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	projectA, execA := seedProjectAndExecution(t, h, "service-a")
	projectB, execB := seedProjectAndExecution(t, h, "service-b")

	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	rec := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name":                 {"Supersale 11.11"},
		"window_start":         {start},
		"window_end":           {end},
		"service_project_id":   {itoa(projectA), itoa(projectB)},
		"service_execution_id": {itoa(execA), itoa(execB)},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create campaign = %d (%s)", rec.Code, rec.Body.String())
	}

	var got campaignRow
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.ID <= 0 || got.TenantID != 7 {
		t.Fatalf("campaign = %+v, want id > 0 and tenant_id 7", got)
	}
	if len(got.Services) != 2 {
		t.Fatalf("services = %+v, want 2", got.Services)
	}
	if got.Active {
		t.Error("a campaign whose window has not started yet must not be active")
	}
}

// The invariant campaignapp.Create enforces: a service's designated
// execution must actually belong to the project it's registered under.
func TestCreateCampaign_RejectsMismatchedExecutionProject(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	_, execA := seedProjectAndExecution(t, h, "service-a")
	projectB, _ := seedProjectAndExecution(t, h, "service-b")

	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	rec := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name":                 {"c"},
		"window_start":         {start},
		"window_end":           {end},
		"service_project_id":   {itoa(projectB)},
		"service_execution_id": {itoa(execA)}, // belongs to a different project
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create campaign (mismatched execution) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateCampaign_RejectsInvalidWindow(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	projectA, execA := seedProjectAndExecution(t, h, "service-a")

	start := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(time.Hour).UTC().Format(time.RFC3339) // before start
	rec := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name":                 {"c"},
		"window_start":         {start},
		"window_end":           {end},
		"service_project_id":   {itoa(projectA)},
		"service_execution_id": {itoa(execA)},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create campaign (inverted window) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateCampaign_RejectsMismatchedServicePairCounts(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	projectA, execA := seedProjectAndExecution(t, h, "service-a")

	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	rec := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name":                 {"c"},
		"window_start":         {start},
		"window_end":           {end},
		"service_project_id":   {itoa(projectA), "999"},
		"service_execution_id": {itoa(execA)}, // one fewer than service_project_id
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create campaign (mismatched pair counts) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListCampaigns_ScopesByTenant(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	projectA, execA := seedProjectAndExecution(t, h, "service-a")

	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	form := url.Values{
		"name": {"c"}, "window_start": {start}, "window_end": {end},
		"service_project_id": {itoa(projectA)}, "service_execution_id": {itoa(execA)},
	}
	if rec := postForm(t, h, "/api/tenants/7/campaigns", form); rec.Code != http.StatusCreated {
		t.Fatalf("create campaign (tenant 7) = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := postForm(t, h, "/api/tenants/9/campaigns", form); rec.Code != http.StatusCreated {
		t.Fatalf("create campaign (tenant 9) = %d (%s)", rec.Code, rec.Body.String())
	}

	rec := do(t, h, http.MethodGet, "/api/tenants/7/campaigns")
	if rec.Code != http.StatusOK {
		t.Fatalf("list campaigns = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []campaignRow
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].TenantID != 7 {
		t.Fatalf("list campaigns(tenant 7) = %+v, want exactly one tenant-7 campaign", got)
	}
}

func TestGetCampaign_RoundTrips(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	projectA, execA := seedProjectAndExecution(t, h, "service-a")

	start := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	create := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name": {"c"}, "window_start": {start}, "window_end": {end},
		"service_project_id": {itoa(projectA)}, "service_execution_id": {itoa(execA)},
	})
	id := decodeID(t, create)

	rec := do(t, h, http.MethodGet, "/api/campaigns/"+itoa(id))
	if rec.Code != http.StatusOK {
		t.Fatalf("get campaign = %d (%s)", rec.Code, rec.Body.String())
	}
	var got campaignRow
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != id {
		t.Fatalf("get campaign id = %d, want %d", got.ID, id)
	}
}

func TestGetCampaign_MissingReturnsNotFound(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	rec := do(t, h, http.MethodGet, "/api/campaigns/999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing campaign = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// pathInt rejects a non-numeric id before touching h.deps.Campaigns at all.
func TestCampaignHandlers_InvalidID_400(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/tenants/x/campaigns"},
		{http.MethodGet, "/api/tenants/x/campaigns"},
		{http.MethodGet, "/api/campaigns/x"},
	}
	for _, tc := range cases {
		if rec := do(t, h, tc.method, tc.path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tc.method, tc.path, rec.Code)
		}
	}
}

func TestCampaignHandlers_CampaignsNotConfigured(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{}) // no Campaigns wired
	rec := do(t, h, http.MethodGet, "/api/tenants/7/campaigns")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list campaigns (not configured) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}
