package httpapi

import (
	"context"
	"net/http"
)

// deployCollection provisions the engines for a collection.
func (h *handlers) deployCollection(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "engines deploying", h.deps.Lifecycle.Deploy)
}

// triggerCollection starts a run across the deployed engines.
func (h *handlers) triggerCollection(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "run triggered", h.deps.Lifecycle.Trigger)
}

// stopCollection halts the in-progress run.
func (h *handlers) stopCollection(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "run stopped", h.deps.Lifecycle.Stop)
}

// purgeCollection stops any run and removes the engines.
func (h *handlers) purgeCollection(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "collection purged", h.deps.Lifecycle.Purge)
}

// lifecycleMutation runs an owner-checked collection operation of the shape
// func(ctx, executionID) error and reports a JSON message on success.
func (h *handlers) lifecycleMutation(w http.ResponseWriter, r *http.Request, msg string, op func(context.Context, int64) error) {
	id, ok := pathInt(r, "collection_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := h.authorizeCollection(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := op(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// collectionStatus reports the deployment/run status of a collection.
func (h *handlers) collectionStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "collection_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	status, err := h.deps.Lifecycle.Status(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// collectionEngines reports the engine pods and ingress of a collection.
func (h *handlers) collectionEngines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "collection_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	c, err := h.deps.Collections.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	detail, err := h.deps.Lifecycle.EnginesDetail(r.Context(), c.ProjectID, id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// planPodLog streams the current logs of a plan's engine pod.
func (h *handlers) planPodLog(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "collection_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	planID, ok := pathInt(r, "plan_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	log, err := h.deps.Lifecycle.PodLog(r.Context(), executionID, planID)
	if err != nil {
		respondError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- Content-Type is text/plain (set above), so the browser does
	// not interpret the pod log as HTML/JS; there is no XSS sink here.
	_, _ = w.Write([]byte(log))
}
