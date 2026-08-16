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

func TestAuthorize_UnknownRoleIgnored(t *testing.T) {
	t.Parallel()
	acct := account.Account{Subject: "u", Global: []string{"ghost"}}
	if d := rbac.Authorize(acct, rbac.DefaultCatalog(), rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionRead}); d.Allowed {
		t.Fatalf("unknown role should not grant: %+v", d)
	}
}
