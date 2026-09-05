package httpapi

import (
	"math"
	"net/http"
	"strconv"

	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/reportapp"
	"github.com/heridotlife/honryu/internal/domain/rbac"
	"github.com/heridotlife/honryu/internal/domain/report"
)

// authorizeReport verifies the caller may read reports derived from
// executionID: rbac.ResourceReport/ActionRead against the execution's own
// tenant (Phase 20 -- report surfaces follow an execution's ownership
// without being its CRUD). In legacy mode there is no account to scope
// by, and report-only deployments need not wire the execution service at
// all, so the route stays open exactly as before -- the campaign/schedule
// gates' precedent.
func (h *handlers) authorizeReport(r *http.Request, executionID int64) error {
	if !h.rbacEnabled() {
		return nil
	}
	c, err := h.deps.Executions.Get(r.Context(), executionID)
	if err != nil {
		return err
	}
	return h.authorize(r.Context(), "", c.TenantID, rbac.ResourceReport, rbac.ActionRead)
}

// authorizeRunShard resolves a run's owning execution and authorizes the
// report read against its tenant. The run row itself carries no tenant,
// so the resolution is part of the authorization. Legacy mode skips both,
// like authorizeReport.
func (h *handlers) authorizeRunShard(r *http.Request, runID int64) error {
	if !h.rbacEnabled() {
		return nil
	}
	executionID, err := h.deps.Lifecycle.RunExecutionID(r.Context(), runID)
	if err != nil {
		return err
	}
	return h.authorizeReport(r, executionID)
}

// runReport returns a run's stored report -- the durable record of what it
// produced, retrievable after the engines and their pods are gone.
// Authorized via the report's own recorded ExecutionID, resolved before
// anything is written back (Phase 20).
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
	if err := h.authorizeReport(r, rep.ExecutionID); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// seriesResponse is the wire shape of GET /api/runs/{run_id}/series: points
// ascending by timestamp, and always an array, so an empty run charts nothing
// rather than null.
type seriesResponse struct {
	Points []reportapp.SeriesPoint `json:"points"`
}

// runSeries returns a run's per-second series -- the shape of the run (VUs,
// RPS, error rate, latency percentiles per second), as distinct from the
// verdicts runReport serves. The unknown-run 404 and the authorization gate
// are runReport's own: the stored report is what makes a run known, and its
// ExecutionID is what the report-read gate checks against. Runs that predate
// the series store have a report and no series, and chart empty.
func (h *handlers) runSeries(w http.ResponseWriter, r *http.Request) {
	if h.deps.Series == nil {
		writeError(w, http.StatusNotFound, "series not configured")
		return
	}
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
	if err := h.authorizeReport(r, rep.ExecutionID); err != nil {
		respondError(w, err)
		return
	}
	intervals, err := h.deps.Series.ListIntervalsByRun(r.Context(), runID)
	if err != nil {
		respondError(w, err)
		return
	}
	points := reportapp.BuildSeries(intervals, reportapp.SeriesPercentiles())
	if points == nil {
		points = []reportapp.SeriesPoint{}
	}
	writeJSON(w, http.StatusOK, seriesResponse{Points: points})
}

// executionReports lists an execution's reports, most recent first, so a
// service owner can see how it has behaved over time.
func (h *handlers) executionReports(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeReport(r, executionID); err != nil {
		respondError(w, err)
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
	if err := h.authorizeReport(r, executionID); err != nil {
		respondError(w, err)
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

type signatureHistoryRowResponse struct {
	Label        string `json:"label"`
	ResponseCode string `json:"response_code,omitempty"`
	Side         string `json:"side"`
	TotalCount   int64  `json:"total_count"`
	RunCount     int    `json:"run_count"`
}

type signatureBreakdownResponse struct {
	Key        string                        `json:"key"`
	TotalCount int64                         `json:"total_count"`
	Rows       []signatureHistoryRowResponse `json:"rows"`
}

type errorSignatureHistoryResponse struct {
	ExecutionID int64                        `json:"execution_id"`
	GroupedBy   string                       `json:"grouped_by"`
	Groups      []signatureBreakdownResponse `json:"groups"`
}

func toErrorSignatureHistoryResponse(executionID int64, by report.SignatureGroupBy, groups []report.SignatureBreakdown) errorSignatureHistoryResponse {
	out := make([]signatureBreakdownResponse, len(groups))
	for i, g := range groups {
		rows := make([]signatureHistoryRowResponse, len(g.Rows))
		for j, r := range g.Rows {
			rows[j] = signatureHistoryRowResponse{
				Label: r.Label, ResponseCode: r.ResponseCode, Side: string(r.Side),
				TotalCount: r.TotalCount, RunCount: r.RunCount,
			}
		}
		out[i] = signatureBreakdownResponse{Key: g.Key, TotalCount: g.TotalCount, Rows: rows}
	}
	return errorSignatureHistoryResponse{ExecutionID: executionID, GroupedBy: string(by), Groups: out}
}

// executionErrorSignatureHistory returns an execution's error signatures
// aggregated across every run, grouped by label (?by=label, the default) or
// by response code (?by=code) -- independently, per the Phase 4 annotation
// that anticipated exactly this. Authorization matches the sibling reports
// routes.
func (h *handlers) executionErrorSignatureHistory(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeReport(r, executionID); err != nil {
		respondError(w, err)
		return
	}
	by := report.GroupByLabel
	if r.URL.Query().Get("by") == "code" {
		by = report.GroupByResponseCode
	}
	rows, err := h.deps.Reports.ErrorSignatureHistory(r.Context(), executionID)
	if err != nil {
		respondError(w, err)
		return
	}
	groups := report.GroupSignatureHistory(rows, by)
	writeJSON(w, http.StatusOK, toErrorSignatureHistoryResponse(executionID, by, groups))
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
	// Bounded before the int conversion below, not merely parsed. pathInt
	// returns an int64 and RunShardKey takes an int, so on a 32-bit build a
	// path like /shard/4294967296 would silently wrap to 0 and serve shard
	// 0's log under a request for a shard that does not exist. Rejecting is
	// the honest answer: a shard index is a small ordinal, and nothing
	// legitimate reaches even MaxInt32.
	shard, ok := pathInt(r, "shard")
	if !ok || shard < 0 || shard > math.MaxInt32 {
		writeError(w, http.StatusBadRequest, "invalid shard")
		return
	}
	// A shard object is run-keyed but owned by the run's execution:
	// authorize report:read against that execution's tenant (Phase 20) --
	// before touching the object store.
	if err := h.authorizeRunShard(r, runID); err != nil {
		respondError(w, err)
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
