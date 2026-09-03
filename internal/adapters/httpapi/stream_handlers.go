package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/heridotlife/honryu/internal/domain/rbac"
)

// streamExecution serves a Server-Sent Events stream of a execution's live
// metrics. Each event is a JSON-encoded engine.Metric. The stream ends when the
// client disconnects.
func (h *handlers) streamExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	// Authorized once at open, before the SSE upgrade writes anything --
	// EventSource cannot send an Authorization header, which is exactly why
	// the session is a cookie (spec constraint). Legacy (no-RBAC)
	// deployments keep the always-open stream they have today.
	if h.rbacEnabled() {
		if err := h.authorizeExecution(r, id, rbac.ActionRead); err != nil {
			respondError(w, err)
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, cancel := h.deps.Events.Subscribe(id)
	defer cancel()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-events:
			if !ok {
				return
			}
			payload, err := json.Marshal(m)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
