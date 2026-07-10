// Package httpapi is the inbound HTTP adapter: it maps REST requests onto the
// application services. Handlers are thin — parse/validate the request, call a
// use-case, and serialize the result. All dependencies are injected via Deps,
// so the router is fully exercised in unit tests with in-memory fakes.
package httpapi

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
)

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Projects *projectapp.Service
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
	mux.HandleFunc("GET /api/projects", h.listProjects)
	mux.HandleFunc("GET /api/projects/{project_id}", h.getProject)
	return mux
}

type handlers struct {
	deps Deps
}

func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
