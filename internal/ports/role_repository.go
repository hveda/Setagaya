package ports

import "context"

// RoleGrant is a single persisted assignment of a role to a subject, optionally
// scoped to a tenant. A nil TenantID is a global (service-provider) grant.
type RoleGrant struct {
	Subject   string
	Email     string
	RoleName  string
	TenantID  *int64
	GrantedBy string
}

// RoleGrants is the resolved set of a subject's persisted grants: global role
// names plus per-tenant role names. It maps directly onto account.Account.
type RoleGrants struct {
	Global  []string
	Tenants map[int64][]string
}

// RoleAssignmentRepository persists role grants for subjects.
type RoleAssignmentRepository interface {
	// AssignRole records a grant. It is idempotent: re-granting the same
	// subject/role/tenant is a no-op rather than an error.
	AssignRole(ctx context.Context, g RoleGrant) error
	// RevokeRole removes a grant. Revoking a grant that does not exist is a
	// no-op. A nil tenantID targets the global grant.
	RevokeRole(ctx context.Context, subject, roleName string, tenantID *int64) error
	// RolesFor resolves all grants held by a subject.
	RolesFor(ctx context.Context, subject string) (RoleGrants, error)
}
