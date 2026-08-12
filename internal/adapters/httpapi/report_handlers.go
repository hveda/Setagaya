package httpapi

import (
	"net/http"
	"strconv"

	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/domain/report"
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

type trendPointResponse struct {
	RunID                    int64   `json:"run_id"`
	Outcome                  string  `json:"outcome"`
	AchievedThroughput       float64 `json:"achieved_throughput"`
	RequestedThroughput      float64 `json:"requested_throughput"`
	ErrorRate                float64 `json:"error_rate"`
	P50                      float64 `json:"p50"`
	P90                      float64 `json:"p90"`
	P95                      float64 `json:"p95"`
	P99                      float64 `json:"p99"`
	HitTargetQPS             bool    `json:"hit_target_qps"`
	HasComparablePredecessor bool    `json:"has_comparable_predecessor"`
	Regressed                bool    `json:"regressed,omitempty"`
}

type trendResponse struct {
	ExecutionID int64                `json:"execution_id"`
	Points      []trendPointResponse `json:"points"`
}

func toTrendResponse(t report.Trend) trendResponse {
	points := make([]trendPointResponse, len(t.Points))
	for i, p := range t.Points {
		points[i] = trendPointResponse{
			RunID: p.RunID, Outcome: string(p.Outcome),
			AchievedThroughput: p.AchievedThroughput, RequestedThroughput: p.RequestedThroughput,
			ErrorRate: p.ErrorRate, P50: p.P50, P90: p.P90, P95: p.P95, P99: p.P99,
			HitTargetQPS: p.HitTargetQPS, HasComparablePredecessor: p.HasComparablePredecessor,
			Regressed: p.Regressed,
		}
	}
	return trendResponse{ExecutionID: t.ExecutionID, Points: points}
}

// executionTrend returns an execution's run-over-run trend: achieved QPS,
// requested QPS, latency percentiles, and error rate as raw advisory series,
// plus the one flagged signal (a run-over-run hit-target-QPS regression
// against its nearest comparable predecessor). Reads via ListReports, the
// same source and ?limit convention executionReports already uses.
func (h *handlers) executionTrend(w http.ResponseWriter, r *http.Request) {
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
	trend := report.BuildTrend(reps)
	trend.ExecutionID = executionID // reps may be empty; the path param is authoritative either way
	writeJSON(w, http.StatusOK, toTrendResponse(trend))
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
