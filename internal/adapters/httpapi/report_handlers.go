package httpapi

import (
	"net/http"
	"strconv"

	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
)

// runReport returns a run's stored report -- the durable record of what it
// produced, retrievable after the engines and their pods are gone.
func (h *handlers) runReport(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathInt(r, "run_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	rep, err := h.deps.Reports.GetReport(r.Context(), runID)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// executionReports lists an execution's reports, most recent first, so a
// service owner can see how it has behaved over time.
func (h *handlers) executionReports(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	reps, err := h.deps.Reports.ListReports(r.Context(), executionID, queryInt(r, "limit"))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reps)
}

// runShardLog serves a shard's captured engine output, durable after the pod
// that produced it is deleted.
func (h *handlers) runShardLog(w http.ResponseWriter, r *http.Request) {
	h.runShardObject(w, r, "log", "text/plain; charset=utf-8")
}

// runShardConfig serves a shard's compiled Taurus config exactly as the run
// used it -- a later re-deploy does not change what this returns.
func (h *handlers) runShardConfig(w http.ResponseWriter, r *http.Request) {
	// text/plain rather than a YAML content-type: what matters is that a
	// browser or curl renders it readably, and there is no registered
	// application/yaml behaviour worth relying on across clients.
	h.runShardObject(w, r, "yml", "text/plain; charset=utf-8")
}

// runShardObject serves the object a run/scenario/shard key addresses. Log and
// config are the only two kinds task 30 exposes; kind is which one, and
// selects the extension the writing side used.
func (h *handlers) runShardObject(w http.ResponseWriter, r *http.Request, kind, contentType string) {
	runID, ok := pathInt(r, "run_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	scenarioID, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	shard, ok := pathInt(r, "shard")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid shard")
		return
	}
	key := lifecycleapp.RunShardKey(runID, scenarioID, int(shard), kind)
	data, err := h.deps.Store.Download(r.Context(), key)
	if err != nil {
		respondError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- Content-Type is text/plain (set above); a captured engine
	// log or config is never interpreted as HTML/JS, so there is no XSS sink.
	_, _ = w.Write(data)
}

// queryInt parses an optional integer query parameter, defaulting to 0
// (ListReports/ReportsSince read that as "no limit") when absent or unparsable
// -- a malformed limit degrading to "everything" is safer than rejecting an
// otherwise valid request over a hint.
func queryInt(r *http.Request, name string) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return 0
	}
	return v
}
