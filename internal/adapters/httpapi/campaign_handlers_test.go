package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
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

func TestGetCampaignVerdict_ReturnsPerServiceOutcomeAndOverallGo(t *testing.T) {
	t.Parallel()
	h, store := newCampaignRouter(t)
	ctx := context.Background()
	projectID, execID := seedProjectAndExecution(t, h, "service-a")

	if err := store.SaveReport(ctx, report.Report{ExecutionID: execID, RunID: 1, Outcome: taurus.OutcomePassed}); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	start := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	create := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name": {"c"}, "window_start": {start}, "window_end": {end},
		"service_project_id": {itoa(projectID)}, "service_execution_id": {itoa(execID)},
	})
	id := decodeID(t, create)

	rec := do(t, h, http.MethodGet, "/api/campaigns/"+itoa(id)+"/verdict")
	if rec.Code != http.StatusOK {
		t.Fatalf("get verdict = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		CampaignID int64 `json:"campaign_id"`
		Go         bool  `json:"go"`
		Services   []struct {
			ExecutionID     int64  `json:"execution_id"`
			HasReport       bool   `json:"has_report"`
			Outcome         string `json:"outcome"`
			FailingCriteria []struct {
				Criterion string `json:"criterion"`
			} `json:"failing_criteria"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.CampaignID != id || !got.Go {
		t.Fatalf("verdict = %+v, want campaign_id %d and go:true", got, id)
	}
	if len(got.Services) != 1 || !got.Services[0].HasReport || got.Services[0].Outcome != "passed" {
		t.Fatalf("verdict services = %+v", got.Services)
	}
	// failing_criteria is omitempty and correctly absent (decodes as nil)
	// for a passed service with nothing to name -- see the failing-service
	// case (verdict_test.go) for the populated shape.
	if len(got.Services[0].FailingCriteria) != 0 {
		t.Fatalf("failing_criteria = %+v, want none for a passed service", got.Services[0].FailingCriteria)
	}
}

func TestGetCampaignVerdict_FailedServiceNamesFailingCriteria(t *testing.T) {
	t.Parallel()
	h, store := newCampaignRouter(t)
	ctx := context.Background()
	projectID, execID := seedProjectAndExecution(t, h, "service-a")

	if err := store.SetExecutionCriteria(ctx, execID, []string{"failures>10%"}); err != nil {
		t.Fatalf("SetExecutionCriteria: %v", err)
	}
	if err := store.SaveReport(ctx, report.Report{
		ExecutionID: execID, RunID: 1, Outcome: taurus.OutcomeFailed, ErrorRate: 0.20,
	}); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	start := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	end := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	create := postForm(t, h, "/api/tenants/7/campaigns", url.Values{
		"name": {"c"}, "window_start": {start}, "window_end": {end},
		"service_project_id": {itoa(projectID)}, "service_execution_id": {itoa(execID)},
	})
	id := decodeID(t, create)

	rec := do(t, h, http.MethodGet, "/api/campaigns/"+itoa(id)+"/verdict")
	if rec.Code != http.StatusOK {
		t.Fatalf("get verdict = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Go       bool `json:"go"`
		Services []struct {
			Outcome         string `json:"outcome"`
			FailingCriteria []struct {
				Criterion string `json:"criterion"`
			} `json:"failing_criteria"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.Go {
		t.Fatal("verdict.go = true, want false -- the service failed")
	}
	if len(got.Services) != 1 || got.Services[0].Outcome != "failed" {
		t.Fatalf("verdict services = %+v", got.Services)
	}
	if len(got.Services[0].FailingCriteria) != 1 || got.Services[0].FailingCriteria[0].Criterion != "failures>10%" {
		t.Fatalf("failing_criteria = %+v, want [failures>10%%]", got.Services[0].FailingCriteria)
	}
}

func TestGetCampaignVerdict_MissingCampaignReturnsNotFound(t *testing.T) {
	t.Parallel()
	h, _ := newCampaignRouter(t)
	rec := do(t, h, http.MethodGet, "/api/campaigns/999/verdict")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get verdict (missing) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}
