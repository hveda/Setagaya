package httpapi

import (
	"context"
	"net/http"
)

// deployExecution provisions the engines for a collection.
func (h *handlers) deployExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "engines deploying", h.deps.Lifecycle.Deploy)
}

// triggerExecution starts a run across the deployed engines.
func (h *handlers) triggerExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "run triggered", h.deps.Lifecycle.Trigger)
}

// stopExecution halts the in-progress run.
func (h *handlers) stopExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "run stopped", h.deps.Lifecycle.Stop)
}

// purgeExecution stops any run and removes the engines.
func (h *handlers) purgeExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "collection purged", h.deps.Lifecycle.Purge)
}

// lifecycleMutation runs an owner-checked collection operation of the shape
// func(ctx, executionID) error and reports a JSON message on success.
func (h *handlers) lifecycleMutation(w http.ResponseWriter, r *http.Request, msg string, op func(context.Context, int64) error) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := h.authorizeExecution(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := op(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// executionStatus reports the deployment/run status of a collection.
func (h *handlers) executionStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
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

// executionEngines reports the engine pods and ingress of a collection.
func (h *handlers) executionEngines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	c, err := h.deps.Executions.Get(r.Context(), id)
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

// scenarioPodLog streams the current logs of a plan's engine pod.
func (h *handlers) scenarioPodLog(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	scenarioID, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	log, err := h.deps.Lifecycle.PodLog(r.Context(), executionID, scenarioID)
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
