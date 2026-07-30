package httpapi

import "net/http"

// adminExecutions lists the executions currently holding engines.
func (h *handlers) adminExecutions(w http.ResponseWriter, r *http.Request) {
	running, err := h.deps.Admin.RunningExecutions(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, running)
}

// adminNodes reports the cluster node pools.
func (h *handlers) adminNodes(w http.ResponseWriter, r *http.Request) {
	pools, err := h.deps.Admin.NodePools(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}
