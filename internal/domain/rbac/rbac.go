// Package rbac holds the pure authorization domain: the resource/action model,
// roles and their permissions, and the decision function that answers whether
// an account may perform an action. No I/O.
package rbac

import "github.com/heridotlife/honryu/internal/domain/account"

// Action is an operation on a resource.
type Action string

// Actions.
const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionList   Action = "list"
	ActionAdmin  Action = "admin"
)

// Resource types.
const (
	ResourceProject   = "project"
	ResourceExecution = "execution"
	ResourceScenario  = "scenario"
	// ResourceRun guards the lifecycle actions (deploy, trigger, stop) as
	// opposed to CRUD on the execution itself. It was named "execution" before
	// the Honryu rename, which now belongs to the aggregate above.
	ResourceRun    = "run"
	ResourceTenant = "tenant"
	ResourceSystem = "system"
)

// Wildcard matches any resource or action.
const Wildcard = "*"

// Standard role names.
const (
	RoleServiceProviderAdmin = "service_provider_admin"
	RoleTenantAdmin          = "tenant_admin"
	RoleTenantEditor         = "tenant_editor"
	RoleTenantViewer         = "tenant_viewer"
)

// Permission grants a set of actions on a resource. A Resource of "*" matches
// any resource; an action "*" matches any action.
type Permission struct {
	Resource string   `json:"resource"`
	Actions  []Action `json:"actions"`
}

// Allows reports whether the permission covers the given resource and action.
func (p Permission) Allows(resource string, action Action) bool {
	if p.Resource != Wildcard && p.Resource != resource {
		return false
	}
	for _, a := range p.Actions {
		if a == Wildcard || a == action {
			return true
		}
	}
	return false
}

// Role is a named bundle of permissions.
type Role struct {
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
	// TenantScoped roles may only be assigned within a tenant, never globally.
	TenantScoped bool `json:"tenant_scoped"`
}

// Can reports whether any of the role's permissions covers resource/action.
func (r Role) Can(resource string, action Action) bool {
	for _, p := range r.Permissions {
		if p.Allows(resource, action) {
			return true
		}
	}
	return false
}

// Request is an authorization query. TenantID is nil for global/unscoped
// resources; when set, tenant-scoped role grants within that tenant apply.
type Request struct {
	Resource string
	Action   Action
	TenantID *int64
}

// Decision is the outcome of Authorize.
type Decision struct {
	Allowed bool
	Reason  string
	// Rule is the role name that granted access, when allowed.
	Rule string
}

// Authorize decides whether acct may perform req, given the role catalog
// (name -> definition). A globally-granted role applies everywhere; a
// tenant-granted role applies only when req.TenantID matches its tenant.
func Authorize(acct account.Account, catalog map[string]Role, req Request) Decision {
	// Global role grants apply to any request.
	for _, name := range acct.Global {
		if role, ok := catalog[name]; ok && role.Can(req.Resource, req.Action) {
			return Decision{Allowed: true, Reason: "granted by global role", Rule: name}
		}
	}
	// Tenant-scoped grants apply only within the request's tenant.
	if req.TenantID != nil {
		for _, name := range acct.RolesInTenant(*req.TenantID) {
			if role, ok := catalog[name]; ok && role.Can(req.Resource, req.Action) {
				return Decision{Allowed: true, Reason: "granted by tenant role", Rule: name}
			}
		}
	}
	return Decision{Allowed: false, Reason: "no role grants this action"}
}

// DefaultCatalog is the built-in role set: a global service-provider admin plus
// tenant-scoped admin/editor/viewer roles.
func DefaultCatalog() map[string]Role {
	all := []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionList, ActionAdmin}
	write := []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionList}
	read := []Action{ActionRead, ActionList}
	return map[string]Role{
		RoleServiceProviderAdmin: {
			Name:        RoleServiceProviderAdmin,
			Permissions: []Permission{{Resource: Wildcard, Actions: []Action{Wildcard}}},
		},
		RoleTenantAdmin: {
			Name:         RoleTenantAdmin,
			TenantScoped: true,
			Permissions: []Permission{
				{Resource: ResourceProject, Actions: all},
				{Resource: ResourceExecution, Actions: all},
				{Resource: ResourceScenario, Actions: all},
				{Resource: ResourceRun, Actions: all},
			},
		},
		RoleTenantEditor: {
			Name:         RoleTenantEditor,
			TenantScoped: true,
			Permissions: []Permission{
				{Resource: ResourceProject, Actions: write},
				{Resource: ResourceExecution, Actions: write},
				{Resource: ResourceScenario, Actions: write},
				{Resource: ResourceRun, Actions: write},
			},
		},
		RoleTenantViewer: {
			Name:         RoleTenantViewer,
			TenantScoped: true,
			Permissions: []Permission{
				{Resource: ResourceProject, Actions: read},
				{Resource: ResourceExecution, Actions: read},
				{Resource: ResourceScenario, Actions: read},
				{Resource: ResourceRun, Actions: read},
			},
		},
	}
}
