package repositorytest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/domain/tenant"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// NewTenantRepo builds a fresh, empty TenantRepository for one test.
type NewTenantRepo func(t *testing.T) ports.TenantRepository

// newTenant constructs a valid tenant, failing the test on a bad fixture.
func newTenant(t *testing.T, name, display string) tenant.Tenant {
	t.Helper()
	tn, err := tenant.New(name, display)
	if err != nil {
		t.Fatalf("tenant.New(%q): %v", name, err)
	}
	return tn
}

// RunTenantRepositoryContract pins tenant CRUD behaviour.
func RunTenantRepositoryContract(t *testing.T, newRepo NewTenantRepo) {
	t.Helper()
	ctx := context.Background()

	t.Run("create, get, list, status", func(t *testing.T) {
		repo := newRepo(t)
		id, err := repo.CreateTenant(ctx, newTenant(t, "acme", "Acme Inc"))
		if err != nil {
			t.Fatalf("CreateTenant: %v", err)
		}
		got, err := repo.GetTenant(ctx, id)
		if err != nil {
			t.Fatalf("GetTenant: %v", err)
		}
		if got.Name != "acme" || got.DisplayName != "Acme Inc" || got.Status != tenant.StatusActive {
			t.Fatalf("tenant = %+v", got)
		}
		if got.CreatedTime.IsZero() {
			t.Fatal("CreatedTime not stamped")
		}

		if _, err := repo.CreateTenant(ctx, newTenant(t, "beta", "Beta")); err != nil {
			t.Fatalf("CreateTenant beta: %v", err)
		}
		list, err := repo.ListTenants(ctx)
		if err != nil {
			t.Fatalf("ListTenants: %v", err)
		}
		if len(list) != 2 || list[0].Name != "acme" || list[1].Name != "beta" {
			t.Fatalf("list = %+v", list)
		}

		if err := repo.SetTenantStatus(ctx, id, tenant.StatusSuspended); err != nil {
			t.Fatalf("SetTenantStatus: %v", err)
		}
		got, _ = repo.GetTenant(ctx, id)
		if got.Status != tenant.StatusSuspended {
			t.Fatalf("status = %q, want suspended", got.Status)
		}
	})

	t.Run("duplicate name rejected", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.CreateTenant(ctx, newTenant(t, "dup", "One")); err != nil {
			t.Fatalf("CreateTenant: %v", err)
		}
		if _, err := repo.CreateTenant(ctx, newTenant(t, "dup", "Two")); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("duplicate = %v, want ErrFileExists", err)
		}
	})

	t.Run("missing tenant is not found", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.GetTenant(ctx, 999); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetTenant(missing) = %v, want ErrNotFound", err)
		}
		if err := repo.SetTenantStatus(ctx, 999, tenant.StatusActive); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("SetTenantStatus(missing) = %v, want ErrNotFound", err)
		}
	})
}

// NewRoleRepo builds a fresh, empty RoleAssignmentRepository for one test.
type NewRoleRepo func(t *testing.T) ports.RoleAssignmentRepository

// RunRoleAssignmentRepositoryContract pins grant/revoke/resolve behaviour.
func RunRoleAssignmentRepositoryContract(t *testing.T, newRepo NewRoleRepo) {
	t.Helper()
	ctx := context.Background()
	tid := int64(7)

	t.Run("assign global and tenant grants, resolve", func(t *testing.T) {
		repo := newRepo(t)
		if err := repo.AssignRole(ctx, ports.RoleGrant{Subject: "alice", Email: "a@x", RoleName: "service_provider_admin", GrantedBy: "root"}); err != nil {
			t.Fatalf("AssignRole global: %v", err)
		}
		if err := repo.AssignRole(ctx, ports.RoleGrant{Subject: "alice", RoleName: "tenant_admin", TenantID: &tid, GrantedBy: "root"}); err != nil {
			t.Fatalf("AssignRole tenant: %v", err)
		}

		got, err := repo.RolesFor(ctx, "alice")
		if err != nil {
			t.Fatalf("RolesFor: %v", err)
		}
		if !slices.Contains(got.Global, "service_provider_admin") {
			t.Fatalf("global = %v", got.Global)
		}
		if !slices.Contains(got.Tenants[tid], "tenant_admin") {
			t.Fatalf("tenant grants = %v", got.Tenants)
		}
	})

	t.Run("assign is idempotent", func(t *testing.T) {
		repo := newRepo(t)
		g := ports.RoleGrant{Subject: "bob", RoleName: "tenant_viewer", TenantID: &tid}
		if err := repo.AssignRole(ctx, g); err != nil {
			t.Fatalf("AssignRole: %v", err)
		}
		if err := repo.AssignRole(ctx, g); err != nil {
			t.Fatalf("AssignRole (again): %v", err)
		}
		got, _ := repo.RolesFor(ctx, "bob")
		if len(got.Tenants[tid]) != 1 {
			t.Fatalf("tenant grants = %v, want one", got.Tenants[tid])
		}
	})

	t.Run("revoke removes only the matching scope", func(t *testing.T) {
		repo := newRepo(t)
		_ = repo.AssignRole(ctx, ports.RoleGrant{Subject: "carol", RoleName: "tenant_editor", TenantID: &tid})
		_ = repo.AssignRole(ctx, ports.RoleGrant{Subject: "carol", RoleName: "tenant_editor"})

		if err := repo.RevokeRole(ctx, "carol", "tenant_editor", &tid); err != nil {
			t.Fatalf("RevokeRole: %v", err)
		}
		got, _ := repo.RolesFor(ctx, "carol")
		if len(got.Tenants[tid]) != 0 {
			t.Fatalf("tenant grant not revoked: %v", got.Tenants[tid])
		}
		if !slices.Contains(got.Global, "tenant_editor") {
			t.Fatalf("global grant wrongly removed: %v", got.Global)
		}

		// Revoking a nonexistent grant is a no-op.
		if err := repo.RevokeRole(ctx, "carol", "nope", nil); err != nil {
			t.Fatalf("RevokeRole(missing): %v", err)
		}
	})

	t.Run("unknown subject resolves empty", func(t *testing.T) {
		repo := newRepo(t)
		got, err := repo.RolesFor(ctx, "nobody")
		if err != nil {
			t.Fatalf("RolesFor: %v", err)
		}
		if len(got.Global) != 0 || len(got.Tenants) != 0 {
			t.Fatalf("expected empty, got %+v", got)
		}
	})
}
