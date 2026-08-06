package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/domain/account"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newAdminRouter(t *testing.T) (http.Handler, *fake.Scheduler) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	sched := fake.NewScheduler()
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))
	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Executions:    executionapp.NewService(store, obj, 100),
		Admin:         adminapp.NewService(store, sched, lifecycle),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})
	return h, sched
}

func TestAbortExecutions_ExecutionList_HappyPath(t *testing.T) {
	t.Parallel()
	h, sched := newAdminRouter(t)
	ctx := context.Background()

	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	executionID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: projectID, ExecutionID: executionID, ScenarioID: 1, Shards: deployShards(1)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	rec := postForm(t, h, "/api/admin/abort", url.Values{"scope": {"execution_list"}, "value": {itoa(executionID)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("abort = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Aborted []int64 `json:"aborted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(got.Aborted) != 1 || got.Aborted[0] != executionID {
		t.Fatalf("aborted = %v, want [%d]", got.Aborted, executionID)
	}

	deployed, _ := sched.DeployedExecutions(ctx, "")
	if _, ok := deployed[executionID]; ok {
		t.Fatal("execution is still deployed after abort")
	}
}

func TestAbortExecutions_InvalidScope(t *testing.T) {
	t.Parallel()
	h, _ := newAdminRouter(t)
	rec := postForm(t, h, "/api/admin/abort", url.Values{"scope": {"bogus"}, "value": {"1"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("abort (bad scope) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestAbortExecutions_Campaign_NothingToAbort(t *testing.T) {
	t.Parallel()
	h, _ := newAdminRouter(t)
	rec := postForm(t, h, "/api/admin/abort", url.Values{"scope": {"campaign"}, "value": {"anything"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("abort (campaign) = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Aborted []int64 `json:"aborted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Aborted) != 0 {
		t.Fatalf("aborted = %v, want none", got.Aborted)
	}
}

// Under RBAC, the kill-switch requires a service-provider admin -- a
// tenant-scoped role, however privileged within its own tenant, must not be
// able to invoke it.
func TestAbortExecutions_RBAC_NonServiceProviderAdminForbidden(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")
	f.prov.Register("val-tok", account.Account{Subject: "val"})
	assignRole(t, f, acme, "val", "tenant_admin")

	rec := f.req(t, http.MethodPost, "/api/admin/abort", "val-tok", url.Values{"scope": {"tenant"}, "value": {itoa(acme)}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("abort (tenant admin, not SP admin) = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
}

// A service-provider admin may invoke the kill-switch, and the action is
// audited.
func TestAbortExecutions_RBAC_ServiceProviderAdminAllowedAndAudited(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")

	pr := f.req(t, http.MethodPost, "/api/projects", "admin-tok", url.Values{"name": {"web"}, "owner": {"admin"}, "tenant_id": {itoa(acme)}})
	projectID := decodeID(t, pr)
	er := f.req(t, http.MethodPost, "/api/executions", "admin-tok", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	executionID := decodeID(t, er)
	if err := f.sched.DeployScenario(context.Background(), ports.DeploySpec{ProjectID: projectID, ExecutionID: executionID, ScenarioID: 1, Shards: deployShards(1)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	rec := f.req(t, http.MethodPost, "/api/admin/abort", "admin-tok", url.Values{"scope": {"execution_list"}, "value": {itoa(executionID)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("abort (SP admin) = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	found := false
	for _, e := range f.audit.Events() {
		if e.Action == "admin.abort" {
			found = true
		}
	}
	if !found {
		t.Fatal("no admin.abort audit event recorded")
	}
}
