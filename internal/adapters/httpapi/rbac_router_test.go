package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	auditmem "github.com/heridotlife/honryu/internal/adapters/audit/memory"
	"github.com/heridotlife/honryu/internal/adapters/auth/token"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/app/authapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/app/tenantapp"
	"github.com/heridotlife/honryu/internal/domain/account"
	"github.com/heridotlife/honryu/internal/domain/rbac"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// rbacFixture wires a full RBAC-enabled router over one shared fake store.
type rbacFixture struct {
	router http.Handler
	store  *fake.Store
	sched  *fake.Scheduler
	prov   *token.Provider
	audit  *auditmem.Log
}

func newRBACFixture(t *testing.T) *rbacFixture {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	sched := fake.NewScheduler()
	prov := token.New()
	// Seed the admin principal and its global grant.
	prov.Register("admin-tok", account.Account{Subject: "admin", Email: "admin@x"})
	if err := store.AssignRole(context.Background(), ports.RoleGrant{
		Subject: "admin", RoleName: rbac.RoleServiceProviderAdmin,
	}); err != nil {
		t.Fatalf("seed admin grant: %v", err)
	}
	auth := authapp.NewService(prov, store, true)
	audit := auditmem.New(nil)
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))
	quota := quotaapp.NewService(store)
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:   projectapp.NewService(store),
		Scenarios:  scenarioapp.NewService(store, obj),
		Executions: executionapp.NewService(store, obj, 100),
		Tenants:    tenantapp.NewService(store, store, store),
		Admin:      adminapp.NewService(store, sched, lifecycle),
		Schedules:  scheduleapp.NewService(store, quota),
		Store:      obj,
		Auth:       auth,
		Audit:      audit,
	})
	return &rbacFixture{router: router, store: store, sched: sched, prov: prov, audit: audit}
}

func (f *rbacFixture) req(t *testing.T, method, path, tok string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, r)
	return rec
}

func TestRBAC_UnauthenticatedRejected(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	rec := f.req(t, http.MethodGet, "/api/projects", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	// Health remains public.
	if rec := f.req(t, http.MethodGet, "/healthz", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
}

func TestRBAC_TenantScopedProjectFiltering(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)

	// Admin creates two tenants.
	acme := createTenant(t, f, "acme", "Acme")
	globex := createTenant(t, f, "globex", "Globex")

	// Admin grants alice editor in acme, bob editor in globex.
	f.prov.Register("alice-tok", account.Account{Subject: "alice"})
	f.prov.Register("bob-tok", account.Account{Subject: "bob"})
	assignRole(t, f, acme, "alice", rbac.RoleTenantEditor)
	assignRole(t, f, globex, "bob", rbac.RoleTenantEditor)

	// Admin creates a project in each tenant.
	createProjectInTenant(t, f, "acme-web", "team-a", acme)
	createProjectInTenant(t, f, "globex-web", "team-b", globex)

	// Administrative actions are audited with the acting subject.
	if events := f.audit.Events(); len(events) == 0 {
		t.Fatal("no audit events recorded")
	} else {
		for _, e := range events {
			if e.Actor != "admin" {
				t.Fatalf("audit actor = %q, want admin (event %+v)", e.Actor, e)
			}
		}
	}

	// Alice sees only acme's project.
	if names := listProjectNames(t, f, "alice-tok"); !equalSet(names, []string{"acme-web"}) {
		t.Fatalf("alice sees %v, want [acme-web]", names)
	}
	// Bob sees only globex's project.
	if names := listProjectNames(t, f, "bob-tok"); !equalSet(names, []string{"globex-web"}) {
		t.Fatalf("bob sees %v, want [globex-web]", names)
	}
	// Admin (service-provider) sees both.
	if names := listProjectNames(t, f, "admin-tok"); !equalSet(names, []string{"acme-web", "globex-web"}) {
		t.Fatalf("admin sees %v, want both", names)
	}
}

func TestRBAC_ViewerDeniedWrite(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")

	f.prov.Register("val-tok", account.Account{Subject: "val"})
	assignRole(t, f, acme, "val", rbac.RoleTenantViewer)

	// A viewer may not create a project in the tenant.
	form := url.Values{"name": {"nope"}, "owner": {"team-a"}, "tenant_id": {strconv.FormatInt(acme, 10)}}
	rec := f.req(t, http.MethodPost, "/api/projects", "val-tok", form)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
}

func TestRBAC_NonAdminCannotManageTenants(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	f.prov.Register("nobody-tok", account.Account{Subject: "nobody"})

	rec := f.req(t, http.MethodPost, "/api/tenants", "nobody-tok",
		url.Values{"name": {"x"}, "display_name": {"X"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin create tenant status = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
}

func TestRBAC_RoleRevocation(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")
	f.prov.Register("carol-tok", account.Account{Subject: "carol"})
	assignRole(t, f, acme, "carol", rbac.RoleTenantEditor)
	createProjectInTenant(t, f, "acme-web", "team-a", acme)

	if names := listProjectNames(t, f, "carol-tok"); len(names) != 1 {
		t.Fatalf("before revoke carol sees %v, want 1", names)
	}

	// Revoke carol's grant.
	path := "/api/tenants/" + strconv.FormatInt(acme, 10) + "/roles?subject=carol&role=" + rbac.RoleTenantEditor
	if rec := f.req(t, http.MethodDelete, path, "admin-tok", nil); rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if names := listProjectNames(t, f, "carol-tok"); len(names) != 0 {
		t.Fatalf("after revoke carol sees %v, want none", names)
	}
}

func TestRBAC_TenantCRUDEndpoints(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	id := createTenant(t, f, "acme", "Acme")

	// List.
	rec := f.req(t, http.MethodGet, "/api/tenants", "admin-tok", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tenants status = %d (%s)", rec.Code, rec.Body.String())
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %v, err = %v", list, err)
	}

	// Get one.
	rec = f.req(t, http.MethodGet, "/api/tenants/"+strconv.FormatInt(id, 10), "admin-tok", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get tenant status = %d (%s)", rec.Code, rec.Body.String())
	}

	// Suspend via PATCH, then confirm.
	rec = f.req(t, http.MethodPatch, "/api/tenants/"+strconv.FormatInt(id, 10), "admin-tok",
		url.Values{"status": {"SUSPENDED"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = f.req(t, http.MethodGet, "/api/tenants/"+strconv.FormatInt(id, 10), "admin-tok", nil)
	var got struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Status != "SUSPENDED" {
		t.Fatalf("status = %q, want SUSPENDED", got.Status)
	}

	// Invalid status is a 400.
	rec = f.req(t, http.MethodPatch, "/api/tenants/"+strconv.FormatInt(id, 10), "admin-tok",
		url.Values{"status": {"BOGUS"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status = %d, want 400", rec.Code)
	}

	// Missing tenant is a 404; malformed id is a 400.
	if rec := f.req(t, http.MethodGet, "/api/tenants/999", "admin-tok", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get missing = %d, want 404", rec.Code)
	}
	if rec := f.req(t, http.MethodGet, "/api/tenants/abc", "admin-tok", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("get malformed id = %d, want 400", rec.Code)
	}
}

func TestRBAC_GlobalRoleEndpoints(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	f.prov.Register("newadmin-tok", account.Account{Subject: "newadmin"})

	// Grant a global service-provider admin role.
	rec := f.req(t, http.MethodPost, "/api/roles", "admin-tok",
		url.Values{"subject": {"newadmin"}, "role": {rbac.RoleServiceProviderAdmin}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign global status = %d (%s)", rec.Code, rec.Body.String())
	}
	// newadmin can now administer tenants.
	if rec := f.req(t, http.MethodGet, "/api/tenants", "newadmin-tok", nil); rec.Code != http.StatusOK {
		t.Fatalf("newadmin list tenants = %d, want 200", rec.Code)
	}

	// Revoke it; newadmin loses admin access.
	rec = f.req(t, http.MethodDelete, "/api/roles?subject=newadmin&role="+rbac.RoleServiceProviderAdmin, "admin-tok", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke global status = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := f.req(t, http.MethodGet, "/api/tenants", "newadmin-tok", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("revoked newadmin list = %d, want 403", rec.Code)
	}
}

func TestRBAC_RoleGrantBadRequests(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")

	// Missing subject on assign.
	rec := f.req(t, http.MethodPost, "/api/tenants/"+strconv.FormatInt(acme, 10)+"/roles", "admin-tok",
		url.Values{"role": {rbac.RoleTenantEditor}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("assign missing subject = %d, want 400", rec.Code)
	}
	// Unknown role → 400.
	rec = f.req(t, http.MethodPost, "/api/tenants/"+strconv.FormatInt(acme, 10)+"/roles", "admin-tok",
		url.Values{"subject": {"x"}, "role": {"wizard"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("assign unknown role = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	// Missing subject/role on revoke.
	rec = f.req(t, http.MethodDelete, "/api/tenants/"+strconv.FormatInt(acme, 10)+"/roles?subject=x", "admin-tok", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke missing role = %d, want 400", rec.Code)
	}
	// Malformed tenant id in path.
	rec = f.req(t, http.MethodPost, "/api/tenants/abc/roles", "admin-tok",
		url.Values{"subject": {"x"}, "role": {rbac.RoleTenantEditor}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("assign malformed tenant id = %d, want 400", rec.Code)
	}
}

func TestRBAC_TenantsNotConfigured(t *testing.T) {
	t.Parallel()
	// A legacy (RBAC-disabled) router without a Tenants service returns 404 for
	// the tenant endpoints rather than pretending they exist.
	router, _ := newTestRouter(t)
	rec := do(t, router, http.MethodGet, "/api/tenants")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("tenants without service = %d, want 404", rec.Code)
	}
}

// --- helpers ---------------------------------------------------------------

func createTenant(t *testing.T, f *rbacFixture, name, display string) int64 {
	t.Helper()
	rec := f.req(t, http.MethodPost, "/api/tenants", "admin-tok",
		url.Values{"name": {name}, "display_name": {display}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tenant %q status = %d (%s)", name, rec.Code, rec.Body.String())
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode tenant: %v", err)
	}
	return out.ID
}

func assignRole(t *testing.T, f *rbacFixture, tenantID int64, subject, role string) {
	t.Helper()
	path := "/api/tenants/" + strconv.FormatInt(tenantID, 10) + "/roles"
	rec := f.req(t, http.MethodPost, path, "admin-tok",
		url.Values{"subject": {subject}, "role": {role}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign %s to %s status = %d (%s)", role, subject, rec.Code, rec.Body.String())
	}
}

func createProjectInTenant(t *testing.T, f *rbacFixture, name, owner string, tenantID int64) {
	t.Helper()
	form := url.Values{"name": {name}, "owner": {owner}, "tenant_id": {strconv.FormatInt(tenantID, 10)}}
	rec := f.req(t, http.MethodPost, "/api/projects", "admin-tok", form)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project %q status = %d (%s)", name, rec.Code, rec.Body.String())
	}
}

func listProjectNames(t *testing.T, f *rbacFixture, tok string) []string {
	t.Helper()
	rec := f.req(t, http.MethodGet, "/api/projects", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list projects status = %d (%s)", rec.Code, rec.Body.String())
	}
	var out []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode projects: %v", err)
	}
	names := make([]string, 0, len(out))
	for _, p := range out {
		names = append(names, p.Name)
	}
	return names
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}

// createProjectInTenantReturningID is createProjectInTenant plus the
// project's assigned id, needed to create an execution under it.
func createProjectInTenantReturningID(t *testing.T, f *rbacFixture, name, owner string, tenantID int64) int64 {
	t.Helper()
	form := url.Values{"name": {name}, "owner": {owner}, "tenant_id": {strconv.FormatInt(tenantID, 10)}}
	rec := f.req(t, http.MethodPost, "/api/projects", "admin-tok", form)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project %q status = %d (%s)", name, rec.Code, rec.Body.String())
	}
	return decodeID(t, rec)
}

// putMultipartAuth is putMultipart (router_phase1_test.go) plus a bearer
// token, needed once RBAC requires every /api/ request to authenticate.
func putMultipartAuth(t *testing.T, f *rbacFixture, path, tok, filename, content string) *httptest.ResponseRecorder {
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

	r := httptest.NewRequest(http.MethodPut, path, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, r)
	return rec
}

// createSchedule's tenant_id is a client-declared value, not derived from
// anything already authorized by authorizeExecution -- authorizeScheduleTenant
// must independently verify the caller is authorized for the specific
// tenant named, the same rule createProject already applies to its own
// client-declared tenant_id. Without it, alice (authorized only in acme, her
// own tenant) could attribute a schedule's quota to globex, a tenant she has
// no relationship to at all.
func TestRBAC_CreateScheduleRequiresAuthorizationForTheDeclaredTenant(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)

	acme := createTenant(t, f, "acme", "Acme")
	globex := createTenant(t, f, "globex", "Globex")
	f.prov.Register("alice-tok", account.Account{Subject: "alice"})
	assignRole(t, f, acme, "alice", rbac.RoleTenantEditor)

	// Admin sets up alice's project, scenario, execution, and load profile in
	// acme -- only the createSchedule call itself is under test.
	projectID := createProjectInTenantReturningID(t, f, "acme-web", "team-a", acme)
	scenarioID := decodeID(t, f.req(t, http.MethodPost, "/api/scenarios", "admin-tok",
		url.Values{"name": {"smoke"}, "project_id": {strconv.FormatInt(projectID, 10)}}))
	executionID := decodeID(t, f.req(t, http.MethodPost, "/api/executions", "admin-tok",
		url.Values{"name": {"peak"}, "project_id": {strconv.FormatInt(projectID, 10)}}))
	configYAML := fmt.Sprintf(`multi-test:
  collectionid: %d
  tests:
    - testid: %d
      concurrency: 10
      rampup: 1
      engines: 2
      duration: 30
`, executionID, scenarioID)
	path := "/api/executions/" + strconv.FormatInt(executionID, 10) + "/config"
	if rec := putMultipartAuth(t, f, path, "admin-tok", "config.yaml", configYAML); rec.Code != http.StatusOK {
		t.Fatalf("upload config = %d (%s)", rec.Code, rec.Body.String())
	}

	fireAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	schedulePath := "/api/executions/" + strconv.FormatInt(executionID, 10) + "/schedules"

	// Alice, authorized only in acme, cannot attribute a schedule to globex.
	rec := f.req(t, http.MethodPost, schedulePath, "alice-tok", url.Values{
		"tenant_id": {strconv.FormatInt(globex, 10)}, "kind": {"one_shot"}, "fire_at": {fireAt},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("alice create schedule for globex = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}

	// Alice can attribute one to acme, her own tenant.
	rec = f.req(t, http.MethodPost, schedulePath, "alice-tok", url.Values{
		"tenant_id": {strconv.FormatInt(acme, 10)}, "kind": {"one_shot"}, "fire_at": {fireAt},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("alice create schedule for acme = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
}
