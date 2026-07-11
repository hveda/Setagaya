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
	"github.com/heridotlife/Setagaya/internal/app/collectionapp"
	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/app/planapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/tenantapp"
	"github.com/heridotlife/Setagaya/internal/app/usageapp"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Projects    *projectapp.Service
	Plans       *planapp.Service
	Collections *collectionapp.Service
	Lifecycle   *lifecycleapp.Service
	Usage       *usageapp.Service
	Admin       *adminapp.Service
	Events      ports.EventBus
	Store       ports.ObjectStore
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

	// Plans
	mux.HandleFunc("POST /api/plans", h.createPlan)
	mux.HandleFunc("GET /api/plans/{plan_id}", h.getPlan)
	mux.HandleFunc("DELETE /api/plans/{plan_id}", h.deletePlan)
	mux.HandleFunc("GET /api/plans/{plan_id}/files", h.listPlanFiles)
	mux.HandleFunc("PUT /api/plans/{plan_id}/files", h.uploadPlanFile)
	mux.HandleFunc("DELETE /api/plans/{plan_id}/files", h.deletePlanFile)

	// Collections
	mux.HandleFunc("POST /api/collections", h.createCollection)
	mux.HandleFunc("GET /api/collections/{collection_id}", h.getCollection)
	mux.HandleFunc("DELETE /api/collections/{collection_id}", h.deleteCollection)
	mux.HandleFunc("GET /api/collections/{collection_id}/files", h.listCollectionFiles)
	mux.HandleFunc("PUT /api/collections/{collection_id}/files", h.uploadCollectionFile)
	mux.HandleFunc("DELETE /api/collections/{collection_id}/files", h.deleteCollectionFile)
	mux.HandleFunc("PUT /api/collections/{collection_id}/config", h.uploadCollectionConfig)
	mux.HandleFunc("GET /api/collections/{collection_id}/config", h.getCollectionConfig)

	// Lifecycle
	mux.HandleFunc("POST /api/collections/{collection_id}/deploy", h.deployCollection)
	mux.HandleFunc("POST /api/collections/{collection_id}/trigger", h.triggerCollection)
	mux.HandleFunc("POST /api/collections/{collection_id}/stop", h.stopCollection)
	mux.HandleFunc("POST /api/collections/{collection_id}/purge", h.purgeCollection)
	mux.HandleFunc("GET /api/collections/{collection_id}/status", h.collectionStatus)
	mux.HandleFunc("GET /api/collections/{collection_id}/engines", h.collectionEngines)
	mux.HandleFunc("GET /api/collections/{collection_id}/plans/{plan_id}/logs", h.planPodLog)
	mux.HandleFunc("GET /api/collections/{collection_id}/stream", h.streamCollection)

	// Usage
	mux.HandleFunc("GET /api/usage/history", h.usageHistory)
	mux.HandleFunc("GET /api/usage/summary", h.usageSummary)

	// Admin
	mux.HandleFunc("GET /api/admin/collections", h.adminCollections)
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
