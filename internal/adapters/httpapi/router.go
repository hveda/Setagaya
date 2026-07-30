// Package httpapi is the inbound HTTP adapter: it maps REST requests onto the
// application services. Handlers are thin — parse/validate the request, call a
// use-case, and serialize the result. All dependencies are injected via Deps,
// so the router is fully exercised in unit tests with in-memory fakes.
package httpapi

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/heridotlife/Setagaya/internal/app/adminapp"
	"github.com/heridotlife/Setagaya/internal/app/authapp"
	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/internal/app/tenantapp"
	"github.com/heridotlife/Setagaya/internal/app/usageapp"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Projects   *projectapp.Service
	Scenarios  *scenarioapp.Service
	Executions *executionapp.Service
	Lifecycle  *lifecycleapp.Service
	Usage      *usageapp.Service
	Admin      *adminapp.Service
	Events     ports.EventBus
	Store      ports.ObjectStore
	// Auth authenticates requests and authorizes actions. When nil or disabled,
	// the legacy no-auth owner path applies (DefaultOwners).
	Auth *authapp.Service
	// Tenants administers tenants and role grants. Required for the /api/tenants
	// endpoints; nil disables them.
	Tenants *tenantapp.Service
	// Audit records administrative actions. Optional; nil disables auditing.
	Audit ports.AuditLog
	// DefaultOwners is the owner set used when RBAC is disabled (no-auth mode).
	DefaultOwners []string
}

// NewRouter builds the HTTP handler for the API server.
func NewRouter(d Deps) http.Handler {
	h := &handlers{deps: d}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.Handle("GET /metrics", promhttp.Handler())

	// Projects
	mux.HandleFunc("GET /api/projects", h.listProjects)
	mux.HandleFunc("POST /api/projects", h.createProject)
	mux.HandleFunc("GET /api/projects/{project_id}", h.getProject)
	mux.HandleFunc("DELETE /api/projects/{project_id}", h.deleteProject)

	// Scenarios
	mux.HandleFunc("POST /api/scenarios", h.createScenario)
	mux.HandleFunc("GET /api/scenarios/{scenario_id}", h.getScenario)
	mux.HandleFunc("DELETE /api/scenarios/{scenario_id}", h.deleteScenario)
	mux.HandleFunc("GET /api/scenarios/{scenario_id}/files", h.listScenarioFiles)
	mux.HandleFunc("PUT /api/scenarios/{scenario_id}/files", h.uploadScenarioFile)
	mux.HandleFunc("DELETE /api/scenarios/{scenario_id}/files", h.deleteScenarioFile)

	// Executions
	mux.HandleFunc("POST /api/executions", h.createExecution)
	mux.HandleFunc("GET /api/executions/{execution_id}", h.getExecution)
	mux.HandleFunc("DELETE /api/executions/{execution_id}", h.deleteExecution)
	mux.HandleFunc("GET /api/executions/{execution_id}/files", h.listExecutionFiles)
	mux.HandleFunc("PUT /api/executions/{execution_id}/files", h.uploadExecutionFile)
	mux.HandleFunc("DELETE /api/executions/{execution_id}/files", h.deleteExecutionFile)
	mux.HandleFunc("PUT /api/executions/{execution_id}/config", h.uploadExecutionConfig)
	mux.HandleFunc("GET /api/executions/{execution_id}/config", h.getExecutionConfig)

	// Lifecycle
	mux.HandleFunc("POST /api/executions/{execution_id}/deploy", h.deployExecution)
	mux.HandleFunc("POST /api/executions/{execution_id}/trigger", h.triggerExecution)
	mux.HandleFunc("POST /api/executions/{execution_id}/stop", h.stopExecution)
	mux.HandleFunc("POST /api/executions/{execution_id}/purge", h.purgeExecution)
	mux.HandleFunc("GET /api/executions/{execution_id}/status", h.executionStatus)
	mux.HandleFunc("GET /api/executions/{execution_id}/engines", h.executionEngines)
	mux.HandleFunc("GET /api/executions/{execution_id}/scenarios/{scenario_id}/logs", h.scenarioPodLog)
	mux.HandleFunc("GET /api/executions/{execution_id}/stream", h.streamExecution)

	// Usage
	mux.HandleFunc("GET /api/usage/history", h.usageHistory)
	mux.HandleFunc("GET /api/usage/summary", h.usageSummary)

	// Admin
	mux.HandleFunc("GET /api/admin/executions", h.adminExecutions)
	mux.HandleFunc("GET /api/admin/nodes", h.adminNodes)

	// Tenants & role grants (multi-tenancy administration)
	mux.HandleFunc("POST /api/tenants", h.createTenant)
	mux.HandleFunc("GET /api/tenants", h.listTenants)
	mux.HandleFunc("GET /api/tenants/{tenant_id}", h.getTenant)
	mux.HandleFunc("PATCH /api/tenants/{tenant_id}", h.setTenantStatus)
	mux.HandleFunc("POST /api/tenants/{tenant_id}/roles", h.assignTenantRole)
	mux.HandleFunc("DELETE /api/tenants/{tenant_id}/roles", h.revokeTenantRole)
	mux.HandleFunc("POST /api/roles", h.assignGlobalRole)
	mux.HandleFunc("DELETE /api/roles", h.revokeGlobalRole)

	// Generic artifact download
	mux.HandleFunc("GET /api/files/{kind}/{id}/{name}", h.downloadFile)

	return h.authenticate(mux)
}

// authenticate is the auth middleware. When RBAC is enabled, it authenticates
// every /api/ request (rejecting unauthenticated callers with 401) and stashes
// the account on the request context for downstream authorization. When RBAC is
// disabled it is a pass-through and the legacy owner checks apply.
func (h *handlers) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.rbacEnabled() && strings.HasPrefix(r.URL.Path, "/api/") {
			acct, err := h.deps.Auth.Authenticate(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			r = r.WithContext(withAccount(r.Context(), acct))
		}
		next.ServeHTTP(w, r)
	})
}

type handlers struct {
	deps Deps
}

func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
