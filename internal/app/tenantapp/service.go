// Package tenantapp is the multi-tenancy administration use-case: it creates and
// lists tenants, changes their lifecycle status, and grants or revokes
// code-defined roles to subjects (globally or scoped to a tenant).
package tenantapp

import (
	"context"
	"errors"

	"github.com/heridotlife/Setagaya/internal/domain/rbac"
	"github.com/heridotlife/Setagaya/internal/domain/tenant"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// ErrUnknownRole is returned when a grant names a role absent from the catalog.
var ErrUnknownRole = errors.New("tenantapp: unknown role")

// ErrGlobalRoleScoped is returned when a tenant-scoped role is granted globally,
// or a global-only role is granted within a tenant.
var ErrGlobalRoleScoped = errors.New("tenantapp: role scope mismatch")

// Service implements the tenant/role administration use-cases.
type Service struct {
	tenants ports.TenantRepository
	roles   ports.RoleAssignmentRepository
	catalog map[string]rbac.Role
}

// NewService wires the tenant service against the default role catalog.
func NewService(tenants ports.TenantRepository, roles ports.RoleAssignmentRepository) *Service {
	return &Service{tenants: tenants, roles: roles, catalog: rbac.DefaultCatalog()}
}

// Create validates and persists a new tenant, returning the stored row.
func (s *Service) Create(ctx context.Context, name, displayName string) (tenant.Tenant, error) {
	tn, err := tenant.New(name, displayName)
	if err != nil {
		return tenant.Tenant{}, err
	}
	id, err := s.tenants.CreateTenant(ctx, tn)
	if err != nil {
		return tenant.Tenant{}, err
	}
	return s.tenants.GetTenant(ctx, id)
}

// Get returns a tenant by id.
func (s *Service) Get(ctx context.Context, id int64) (tenant.Tenant, error) {
	return s.tenants.GetTenant(ctx, id)
}

// List returns all tenants.
func (s *Service) List(ctx context.Context) ([]tenant.Tenant, error) {
	return s.tenants.ListTenants(ctx)
}

// SetStatus validates and updates a tenant's lifecycle status.
func (s *Service) SetStatus(ctx context.Context, id int64, status string) error {
	if status != tenant.StatusActive && status != tenant.StatusSuspended {
		return tenant.ErrStatusInvalid
	}
	return s.tenants.SetTenantStatus(ctx, id, status)
}

// AssignRole grants a role to a subject after checking the role exists, its
// scope matches (tenant-scoped roles need a tenant, global roles must not have
// one), and any named tenant exists.
func (s *Service) AssignRole(ctx context.Context, g ports.RoleGrant) error {
	if err := s.validateGrant(ctx, g); err != nil {
		return err
	}
	return s.roles.AssignRole(ctx, g)
}

// RevokeRole removes a role grant from a subject.
func (s *Service) RevokeRole(ctx context.Context, subject, roleName string, tenantID *int64) error {
	return s.roles.RevokeRole(ctx, subject, roleName, tenantID)
}

// RolesFor resolves the grants held by a subject.
func (s *Service) RolesFor(ctx context.Context, subject string) (ports.RoleGrants, error) {
	return s.roles.RolesFor(ctx, subject)
}

func (s *Service) validateGrant(ctx context.Context, g ports.RoleGrant) error {
	role, ok := s.catalog[g.RoleName]
	if !ok {
		return ErrUnknownRole
	}
	if role.TenantScoped && g.TenantID == nil {
		return ErrGlobalRoleScoped
	}
	if !role.TenantScoped && g.TenantID != nil {
		return ErrGlobalRoleScoped
	}
	if g.TenantID != nil {
		if _, err := s.tenants.GetTenant(ctx, *g.TenantID); err != nil {
			return err
		}
	}
	return nil
}
