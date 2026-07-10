// Package httpapi is the inbound HTTP adapter: it maps REST requests onto the
// application services. Handlers are thin — parse/validate the request, call a
// use-case, and serialize the result. All dependencies are injected via Deps,
// so the router is fully exercised in unit tests with in-memory fakes.
package httpapi

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/hveda/Setagaya/v3/internal/app/collectionapp"
	"github.com/hveda/Setagaya/v3/internal/app/planapp"
	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Projects    *projectapp.Service
	Plans       *planapp.Service
	Collections *collectionapp.Service
	Store       ports.ObjectStore
	// DefaultOwners is the owner set used when no authenticated account is
	// present (no-auth mode). Replaced by the auth adapter in a later phase.
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

	// Generic artifact download
	mux.HandleFunc("GET /api/files/{kind}/{id}/{name}", h.downloadFile)

	return mux
}

type handlers struct {
	deps Deps
}

func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
