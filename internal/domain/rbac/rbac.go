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
	// ResourceCampaign guards campaign create/read/update/list/admin --
	// separate from ResourceProject/ResourceExecution because a campaign
	// manager can register any project in their tenant into a campaign
	// without holding edit rights on that project itself.
	ResourceCampaign = "campaign"
	// ResourceSchedule guards an execution's schedule -- and a reservation,
	// which is scheduled capacity, materialized, so it does not earn a
	// separate resource. Split from ResourceExecution so a campaign manager
	// can see every participating tenant's plan (to run the coordination
	// meeting) without holding write on the executions themselves.
	ResourceSchedule = "schedule"
	// ResourceReport guards execution/run reports, trend, error-signature
	// history, and shard log/config -- read surfaces that follow an
	// execution's ownership but are not the execution CRUD itself.
	ResourceReport = "report"
)

// Wildcard matches any resource or action.
const Wildcard = "*"

// Standard role names.
const (
	RoleServiceProviderAdmin = "service_provider_admin"
	RoleTenantAdmin          = "tenant_admin"
	RoleTenantEditor         = "tenant_editor"
	RoleTenantViewer         = "tenant_viewer"
	// RoleCampaignManager is a PM's role: it can create and manage
	// campaigns within its tenant, and read (but not edit) any project or
	// execution there to see what it's binding into one -- deliberately
	// separate from RoleTenantAdmin/Editor, since a campaign freezes other
	// teams' work and that authority should not be bundled with ordinary
	// project edit rights.
	RoleCampaignManager = "campaign_manager"
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
				{Resource: ResourceSchedule, Actions: all},
				{Resource: ResourceReport, Actions: all},
				// Scoped to the request's own tenant like every other
				// permission on this role -- Authorize only ever consults
				// this grant when req.TenantID matches a tenant the account
				// holds RoleTenantAdmin in, never globally. Without this a
				// tenant admin cannot reach its own tenant:admin-gated
				// routes (tenantAdminGate), including the reservation
				// calendar the Reservations page depends on.
				{Resource: ResourceTenant, Actions: []Action{ActionAdmin}},
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
				{Resource: ResourceSchedule, Actions: write},
				{Resource: ResourceReport, Actions: read},
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
				{Resource: ResourceSchedule, Actions: read},
				{Resource: ResourceReport, Actions: read},
			},
		},
		RoleCampaignManager: {
			Name:         RoleCampaignManager,
			TenantScoped: true,
			Permissions: []Permission{
				{Resource: ResourceCampaign, Actions: all},
				{Resource: ResourceProject, Actions: read},
				{Resource: ResourceExecution, Actions: read},
				// Coordination happens in a meeting, outside Honryu -- a PM
				// needs to SEE every participating tenant's plan to run
				// that meeting and pull its report, never to change it.
				// Write arrives only by composition: a separate
				// RoleTenantEditor grant in a tenant this PM owns.
				{Resource: ResourceSchedule, Actions: read},
				{Resource: ResourceReport, Actions: read},
			},
		},
	}
}
