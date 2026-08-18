package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/heridotlife/honryu/internal/domain/run"
)

// deployExecution provisions the engines for a execution.
func (h *handlers) deployExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "engines deploying", h.deps.Lifecycle.Deploy)
}

// triggerExecution starts a run across the deployed engines.
//
// Deploy returning success only means the pods were created, not that they are
// scheduled, image-pulled, and running — so an immediate Trigger used to 409
// until the caller's own retry happened to land after readiness (every human
// and script client re-implemented that retry; task 121 hit it live). The
// handler now owns the bounded wait calibrationapp has had since Phase 7:
// retry the readiness-class conflicts for up to Deps.TriggerReadyTimeout,
// then surface the last error exactly as before.
func (h *handlers) triggerExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, id); err != nil {
		respondError(w, err)
		return
	}

	poll := h.deps.TriggerReadyPoll
	if poll <= 0 {
		poll = defaultTriggerReadyPoll
	}
	timeout := h.deps.TriggerReadyTimeout
	if timeout <= 0 {
		timeout = defaultTriggerReadyTimeout
	}

	deadline := time.Now().Add(timeout)
	for {
		err := h.deps.Lifecycle.Trigger(r.Context(), id)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]string{"message": "run triggered"})
			return
		}
		// Only the readiness races are worth waiting out. Everything else —
		// already running, frozen campaign, quota, compile errors — is
		// returned immediately; retrying those just delays the answer.
		if !errors.Is(err, run.ErrNotDeployed) && !errors.Is(err, run.ErrEnginesNotReady) {
			respondError(w, err)
			return
		}
		if time.Now().After(deadline) || r.Context().Err() != nil {
			respondError(w, err)
			return
		}
		// A poll capped at the remaining budget so the final attempt cannot
		// sleep past the deadline and add the poll interval to the latency.
		if remaining := time.Until(deadline); remaining < poll {
			poll = remaining
		}
		h.sleep(poll)
	}
}

// stopExecution halts the in-progress run.
func (h *handlers) stopExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "run stopped", h.deps.Lifecycle.Stop)
}

// purgeExecution stops any run and removes the engines.
func (h *handlers) purgeExecution(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMutation(w, r, "execution purged", h.deps.Lifecycle.Purge)
}

// lifecycleMutation runs an owner-checked execution operation of the shape
// func(ctx, executionID) error and reports a JSON message on success.
func (h *handlers) lifecycleMutation(w http.ResponseWriter, r *http.Request, msg string, op func(context.Context, int64) error) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
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

// executionStatus reports the deployment/run status of a execution.
func (h *handlers) executionStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	status, err := h.deps.Lifecycle.Status(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// executionEngines reports the engine pods and ingress of a execution.
func (h *handlers) executionEngines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
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

// scenarioPodLog streams the current logs of a scenario's engine pod.
func (h *handlers) scenarioPodLog(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	scenarioID, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
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
