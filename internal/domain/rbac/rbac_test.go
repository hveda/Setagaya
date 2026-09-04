package rbac_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/domain/account"
	"github.com/heridotlife/honryu/internal/domain/rbac"
)

func ptr(v int64) *int64 { return &v }

func TestPermission_Allows(t *testing.T) {
	t.Parallel()
	cases := []struct {
		perm     rbac.Permission
		resource string
		action   rbac.Action
		want     bool
	}{
		{rbac.Permission{Resource: "project", Actions: []rbac.Action{rbac.ActionRead}}, "project", rbac.ActionRead, true},
		{rbac.Permission{Resource: "project", Actions: []rbac.Action{rbac.ActionRead}}, "project", rbac.ActionDelete, false},
		{rbac.Permission{Resource: "project", Actions: []rbac.Action{rbac.ActionRead}}, "scenario", rbac.ActionRead, false},
		{rbac.Permission{Resource: "*", Actions: []rbac.Action{rbac.ActionRead}}, "anything", rbac.ActionRead, true},
		{rbac.Permission{Resource: "project", Actions: []rbac.Action{"*"}}, "project", rbac.ActionDelete, true},
	}
	for i, c := range cases {
		if got := c.perm.Allows(c.resource, c.action); got != c.want {
			t.Errorf("case %d: Allows = %v, want %v", i, got, c.want)
		}
	}
}

func TestAuthorize_GlobalRole(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	acct := account.Account{Subject: "sp", Global: []string{rbac.RoleServiceProviderAdmin}}

	// Global admin can do anything, tenant-scoped or not.
	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionDelete, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("global admin denied: %+v", d)
	}
	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceSystem, Action: rbac.ActionAdmin}); !d.Allowed || d.Rule != rbac.RoleServiceProviderAdmin {
		t.Fatalf("global admin on system = %+v", d)
	}
}

func TestAuthorize_TenantScoped(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	acct := account.Account{
		Subject: "u",
		Tenants: map[int64][]string{5: {rbac.RoleTenantEditor}},
	}

	// Editor may create within tenant 5.
	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionCreate, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("editor create in tenant 5 denied: %+v", d)
	}
	// But not in tenant 9 (no roles there).
	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionCreate, TenantID: ptr(9)}); d.Allowed {
		t.Fatalf("editor should be denied in tenant 9: %+v", d)
	}
	// And not on a global (nil-tenant) request.
	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionCreate}); d.Allowed {
		t.Fatalf("tenant role should not grant global: %+v", d)
	}
}

func TestAuthorize_ViewerCannotWrite(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	acct := account.Account{Subject: "v", Tenants: map[int64][]string{1: {rbac.RoleTenantViewer}}}

	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceExecution, Action: rbac.ActionRead, TenantID: ptr(1)}); !d.Allowed {
		t.Fatalf("viewer read denied: %+v", d)
	}
	if d := rbac.Authorize(acct, catalog, rbac.Request{Resource: rbac.ResourceExecution, Action: rbac.ActionDelete, TenantID: ptr(1)}); d.Allowed {
		t.Fatalf("viewer delete should be denied: %+v", d)
	}
}

// A campaign manager freezes other teams' projects for a window, so this
// authority is deliberately not bundled into RoleTenantEditor -- an editor
// with no campaign_manager grant must be denied, even though editor already
// grants ResourceProject/ResourceExecution write access.
func TestAuthorize_CampaignManager(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	editor := account.Account{Subject: "e", Tenants: map[int64][]string{5: {rbac.RoleTenantEditor}}}
	if d := rbac.Authorize(editor, catalog, rbac.Request{Resource: rbac.ResourceCampaign, Action: rbac.ActionCreate, TenantID: ptr(5)}); d.Allowed {
		t.Fatalf("tenant editor should not be able to create a campaign: %+v", d)
	}

	pm := account.Account{Subject: "pm", Tenants: map[int64][]string{5: {rbac.RoleCampaignManager}}}
	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceCampaign, Action: rbac.ActionCreate, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("campaign manager create denied: %+v", d)
	}
	// Not in a different tenant.
	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceCampaign, Action: rbac.ActionCreate, TenantID: ptr(9)}); d.Allowed {
		t.Fatalf("campaign manager should be denied in a tenant they hold no grant in: %+v", d)
	}
	// Can read projects/executions (to see what they're binding), but not edit them.
	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionRead, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("campaign manager project read denied: %+v", d)
	}
	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionUpdate, TenantID: ptr(5)}); d.Allowed {
		t.Fatalf("campaign manager should not be able to edit a project: %+v", d)
	}
}

// Phase 20 resource-model surgery: tenant_admin gains ResourceTenant/admin
// (bug 3 in the spec -- without this a tenant admin cannot reach its own
// tenant:admin-gated routes, including the Reservations page's calendar).
func TestAuthorize_TenantAdminCanAdministerOwnTenant(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	admin := account.Account{Subject: "a", Tenants: map[int64][]string{5: {rbac.RoleTenantAdmin}}}

	if d := rbac.Authorize(admin, catalog, rbac.Request{Resource: rbac.ResourceTenant, Action: rbac.ActionAdmin, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("tenant admin denied admin on its own tenant: %+v", d)
	}
	// Not a different tenant -- the grant is scoped, not global.
	if d := rbac.Authorize(admin, catalog, rbac.Request{Resource: rbac.ResourceTenant, Action: rbac.ActionAdmin, TenantID: ptr(9)}); d.Allowed {
		t.Fatalf("tenant admin should not administer a tenant it holds no grant in: %+v", d)
	}
}

// Phase 20: ResourceSchedule and ResourceReport give tenant_viewer read
// access without granting write -- the two new resources close the gap that
// left ResourceExecution/Run/Scenario asked about by zero handlers.
func TestAuthorize_ScheduleAndReport(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	viewer := account.Account{Subject: "v", Tenants: map[int64][]string{1: {rbac.RoleTenantViewer}}}
	editor := account.Account{Subject: "e", Tenants: map[int64][]string{1: {rbac.RoleTenantEditor}}}

	if d := rbac.Authorize(viewer, catalog, rbac.Request{Resource: rbac.ResourceSchedule, Action: rbac.ActionRead, TenantID: ptr(1)}); !d.Allowed {
		t.Fatalf("viewer schedule read denied: %+v", d)
	}
	if d := rbac.Authorize(viewer, catalog, rbac.Request{Resource: rbac.ResourceSchedule, Action: rbac.ActionCreate, TenantID: ptr(1)}); d.Allowed {
		t.Fatalf("viewer should not create a schedule: %+v", d)
	}
	if d := rbac.Authorize(viewer, catalog, rbac.Request{Resource: rbac.ResourceReport, Action: rbac.ActionRead, TenantID: ptr(1)}); !d.Allowed {
		t.Fatalf("viewer report read denied: %+v", d)
	}
	if d := rbac.Authorize(editor, catalog, rbac.Request{Resource: rbac.ResourceSchedule, Action: rbac.ActionCreate, TenantID: ptr(1)}); !d.Allowed {
		t.Fatalf("editor schedule create denied: %+v", d)
	}
}

// Phase 20's binding decision: the PM gets no write access anywhere except
// campaigns, even after gaining ResourceSchedule/ResourceReport read. Write
// arrives only by composition with a separate tenant grant (covered by
// TestAuthorize_CampaignManager above for project/execution).
func TestAuthorize_CampaignManagerScheduleReadOnly(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	pm := account.Account{Subject: "pm", Tenants: map[int64][]string{5: {rbac.RoleCampaignManager}}}

	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceSchedule, Action: rbac.ActionRead, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("campaign manager schedule read denied: %+v", d)
	}
	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceSchedule, Action: rbac.ActionCreate, TenantID: ptr(5)}); d.Allowed {
		t.Fatalf("campaign manager should not create a schedule: %+v", d)
	}
	if d := rbac.Authorize(pm, catalog, rbac.Request{Resource: rbac.ResourceReport, Action: rbac.ActionRead, TenantID: ptr(5)}); !d.Allowed {
		t.Fatalf("campaign manager report read denied: %+v", d)
	}
}

// Phase 20's catalog table, one case per added or changed
// Role.Can(resource, action) combination (spec Approach B), including the
// negatives: campaign_manager gains no write anywhere outside campaign, and
// report stays read+list for every role but admin.
func TestDefaultCatalog_Phase20Grants(t *testing.T) {
	t.Parallel()
	catalog := rbac.DefaultCatalog()
	cases := []struct {
		role     string
		resource string
		action   rbac.Action
		want     bool
	}{
		// tenant_admin += tenant:admin (bug 3), plus the two new resources
		// at full strength -- extracting schedule/report into their own
		// resources must not demote the tenant's admin below its editor.
		{rbac.RoleTenantAdmin, rbac.ResourceTenant, rbac.ActionAdmin, true},
		{rbac.RoleTenantAdmin, rbac.ResourceTenant, rbac.ActionUpdate, false},
		{rbac.RoleTenantAdmin, rbac.ResourceSchedule, rbac.ActionCreate, true},
		{rbac.RoleTenantAdmin, rbac.ResourceReport, rbac.ActionRead, true},
		// tenant_editor += schedule:write, report:read+list.
		{rbac.RoleTenantEditor, rbac.ResourceSchedule, rbac.ActionCreate, true},
		{rbac.RoleTenantEditor, rbac.ResourceSchedule, rbac.ActionDelete, true},
		{rbac.RoleTenantEditor, rbac.ResourceReport, rbac.ActionRead, true},
		{rbac.RoleTenantEditor, rbac.ResourceReport, rbac.ActionList, true},
		{rbac.RoleTenantEditor, rbac.ResourceReport, rbac.ActionUpdate, false},
		// tenant_viewer += schedule:read+list, report:read+list.
		{rbac.RoleTenantViewer, rbac.ResourceSchedule, rbac.ActionRead, true},
		{rbac.RoleTenantViewer, rbac.ResourceSchedule, rbac.ActionList, true},
		{rbac.RoleTenantViewer, rbac.ResourceSchedule, rbac.ActionCreate, false},
		{rbac.RoleTenantViewer, rbac.ResourceReport, rbac.ActionRead, true},
		{rbac.RoleTenantViewer, rbac.ResourceReport, rbac.ActionList, true},
		{rbac.RoleTenantViewer, rbac.ResourceReport, rbac.ActionDelete, false},
		// campaign_manager += schedule:read+list, report:read+list -- and
		// no write outside campaign, the phase's binding decision.
		{rbac.RoleCampaignManager, rbac.ResourceSchedule, rbac.ActionRead, true},
		{rbac.RoleCampaignManager, rbac.ResourceSchedule, rbac.ActionList, true},
		{rbac.RoleCampaignManager, rbac.ResourceSchedule, rbac.ActionCreate, false},
		{rbac.RoleCampaignManager, rbac.ResourceSchedule, rbac.ActionUpdate, false},
		{rbac.RoleCampaignManager, rbac.ResourceReport, rbac.ActionRead, true},
		{rbac.RoleCampaignManager, rbac.ResourceReport, rbac.ActionDelete, false},
		{rbac.RoleCampaignManager, rbac.ResourceCampaign, rbac.ActionUpdate, true},
		{rbac.RoleCampaignManager, rbac.ResourceExecution, rbac.ActionUpdate, false},
	}
	for _, c := range cases {
		role, ok := catalog[c.role]
		if !ok {
			t.Fatalf("role %q missing from catalog", c.role)
		}
		if got := role.Can(c.resource, c.action); got != c.want {
			t.Errorf("%s.Can(%s, %s) = %v, want %v", c.role, c.resource, c.action, got, c.want)
		}
	}
}

func TestAuthorize_UnknownRoleIgnored(t *testing.T) {
	t.Parallel()
	acct := account.Account{Subject: "u", Global: []string{"ghost"}}
	if d := rbac.Authorize(acct, rbac.DefaultCatalog(), rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionRead}); d.Allowed {
		t.Fatalf("unknown role should not grant: %+v", d)
	}
}
