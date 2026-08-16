package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// maxIngestBytes bounds a batch. A pod pushing every second sends far less than
// this; the cap exists so a misbehaving or hostile client cannot exhaust memory.
const maxIngestBytes = 8 << 20 // 8 MiB

// ingest accepts one engine pod's measurements.
//
// This is the only inbound path for results. Measurements are pushed by a
// sidecar rather than scraped, so the control plane never has to reach into a
// cluster -- which is what lets an execution run somewhere it could not address.
func (h *handlers) ingest(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeIngest(r) {
		writeError(w, http.StatusUnauthorized, "invalid ingest credentials")
		return
	}

	var batch metrics.Batch
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestBytes))
	if err := dec.Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, "malformed batch")
		return
	}
	if batch.ExecutionID <= 0 {
		writeError(w, http.StatusBadRequest, "batch does not name an execution")
		return
	}

	if err := h.deps.Metrics.Ingest(r.Context(), batch); err != nil {
		respondError(w, err)
		return
	}
	// Nothing to return: the pod has no use for a body, and a smaller response
	// matters when every pod posts every second.
	w.WriteHeader(http.StatusAccepted)
}

// authorizeIngest checks the shared engine credential.
//
// Engine pods are not users: they hold no account and outlive no session, so
// they authenticate with a token issued to the deployment rather than through
// the user AuthProvider. An empty configured token rejects everything, so a
// deployment that forgot to set one is closed rather than open.
func (h *handlers) authorizeIngest(r *http.Request) bool {
	if h.deps.IngestToken == "" {
		return false
	}
	presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	// Constant time, so a wrong token cannot be discovered a byte at a time.
	return subtle.ConstantTimeCompare([]byte(presented), []byte(h.deps.IngestToken)) == 1
}
