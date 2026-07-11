package httpapi

import "net/http"

// adminCollections lists the collections currently holding engines.
func (h *handlers) adminCollections(w http.ResponseWriter, r *http.Request) {
	running, err := h.deps.Admin.RunningCollections(r.Context())
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
