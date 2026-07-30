package tenantapp_test

import (
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/app/tenantapp"
	"github.com/heridotlife/honryu/internal/domain/rbac"
	"github.com/heridotlife/honryu/internal/domain/tenant"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newSvc(t *testing.T) (*tenantapp.Service, *fake.Store) {
	t.Helper()
	store := fake.NewStore()
	return tenantapp.NewService(store, store), store
}

func TestCreateAndList(t *testing.T) {
	t.Parallel()
	svc, _ := newSvc(t)
	ctx := t.Context()

	tn, err := svc.Create(ctx, "Acme", "Acme Inc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tn.ID == 0 || tn.Name != "acme" || tn.Status != tenant.StatusActive {
		t.Fatalf("created tenant = %+v", tn)
	}

	got, err := svc.Get(ctx, tn.ID)
	if err != nil || got.Name != "acme" {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
}

func TestCreateRejectsInvalid(t *testing.T) {
	t.Parallel()
	svc, _ := newSvc(t)
	if _, err := svc.Create(t.Context(), "ab", "too short"); !errors.Is(err, tenant.ErrNameInvalid) {
		t.Fatalf("Create(short) = %v, want ErrNameInvalid", err)
	}
}

func TestSetStatus(t *testing.T) {
	t.Parallel()
	svc, _ := newSvc(t)
	ctx := t.Context()
	tn, _ := svc.Create(ctx, "acme", "Acme")

	if err := svc.SetStatus(ctx, tn.ID, tenant.StatusSuspended); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, _ := svc.Get(ctx, tn.ID)
	if got.Active() {
		t.Fatal("tenant still active after suspend")
	}

	if err := svc.SetStatus(ctx, tn.ID, "BOGUS"); !errors.Is(err, tenant.ErrStatusInvalid) {
		t.Fatalf("SetStatus(bogus) = %v, want ErrStatusInvalid", err)
	}
}

func TestAssignRole(t *testing.T) {
	t.Parallel()
	svc, _ := newSvc(t)
	ctx := t.Context()
	tn, _ := svc.Create(ctx, "acme", "Acme")

	// Tenant-scoped role within a real tenant.
	if err := svc.AssignRole(ctx, ports.RoleGrant{Subject: "alice", RoleName: rbac.RoleTenantAdmin, TenantID: &tn.ID}); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	got, _ := svc.RolesFor(ctx, "alice")
	if len(got.Tenants[tn.ID]) != 1 {
		t.Fatalf("grants = %+v", got)
	}

	// Global role granted globally.
	if err := svc.AssignRole(ctx, ports.RoleGrant{Subject: "root", RoleName: rbac.RoleServiceProviderAdmin}); err != nil {
		t.Fatalf("AssignRole global: %v", err)
	}

	if err := svc.RevokeRole(ctx, "alice", rbac.RoleTenantAdmin, &tn.ID); err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	got, _ = svc.RolesFor(ctx, "alice")
	if len(got.Tenants[tn.ID]) != 0 {
		t.Fatalf("grant not revoked: %+v", got)
	}
}

func TestAssignRoleValidation(t *testing.T) {
	t.Parallel()
	svc, _ := newSvc(t)
	ctx := t.Context()
	tn, _ := svc.Create(ctx, "acme", "Acme")
	missing := int64(999)

	tests := []struct {
		name string
		g    ports.RoleGrant
		want error
	}{
		{"unknown role", ports.RoleGrant{Subject: "a", RoleName: "wizard", TenantID: &tn.ID}, tenantapp.ErrUnknownRole},
		{"tenant role granted globally", ports.RoleGrant{Subject: "a", RoleName: rbac.RoleTenantEditor}, tenantapp.ErrGlobalRoleScoped},
		{"global role granted in tenant", ports.RoleGrant{Subject: "a", RoleName: rbac.RoleServiceProviderAdmin, TenantID: &tn.ID}, tenantapp.ErrGlobalRoleScoped},
		{"tenant does not exist", ports.RoleGrant{Subject: "a", RoleName: rbac.RoleTenantEditor, TenantID: &missing}, ports.ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.AssignRole(ctx, tc.g); !errors.Is(err, tc.want) {
				t.Fatalf("AssignRole = %v, want %v", err, tc.want)
			}
		})
	}
}
