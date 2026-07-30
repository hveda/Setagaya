// Package prometheus implements ports.MetricsSink over the Prometheus client,
// exposing the same series as v2 (latency summaries, status counter, threads
// gauge) so the carried-over Grafana dashboards keep working.
package prometheus

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Sink records engine metrics into Prometheus vectors.
type Sink struct {
	collectionLatency *prometheus.SummaryVec
	planLatency       *prometheus.SummaryVec
	labelLatency      *prometheus.SummaryVec
	statusCounter     *prometheus.CounterVec
	threadsGauge      *prometheus.GaugeVec
}

var _ ports.MetricsSink = (*Sink)(nil)

// objectives match v2: p90 and p99 percentile latency.
var objectives = map[float64]float64{0.9: 0.01, 0.99: 0.001}

// New builds a Sink and registers its collectors with reg (use
// prometheus.DefaultRegisterer for the default /metrics endpoint).
func New(reg prometheus.Registerer) *Sink {
	s := &Sink{
		collectionLatency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: "honryu", Name: "latency_execution",
			Help: "Percentile latency of a collection", Objectives: objectives,
		}, []string{"execution_id", "run_id"}),
		planLatency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: "honryu", Name: "latency_scenario",
			Help: "Percentile latency of a plan", Objectives: objectives,
		}, []string{"execution_id", "scenario_id", "run_id"}),
		labelLatency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: "honryu", Name: "latency_label",
			Help: "Percentile latency of a label", Objectives: objectives,
		}, []string{"execution_id", "label", "run_id"}),
		statusCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "honryu", Name: "status_counter",
			Help: "Count of responses grouped by response code",
		}, []string{"execution_id", "scenario_id", "run_id", "engine_no", "label", "status"}),
		threadsGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "honryu", Name: "threads_gauge",
			Help: "Current number of threads running in the engine",
		}, []string{"execution_id", "scenario_id", "run_id", "engine_no"}),
	}
	reg.MustRegister(s.collectionLatency, s.planLatency, s.labelLatency, s.statusCounter, s.threadsGauge)
	return s
}

// Record ingests one measurement into the latency, status, and threads series.
func (s *Sink) Record(m engine.Metric) {
	s.collectionLatency.WithLabelValues(m.ExecutionID, m.RunID).Observe(m.Latency)
	s.planLatency.WithLabelValues(m.ExecutionID, m.ScenarioID, m.RunID).Observe(m.Latency)
	s.labelLatency.WithLabelValues(m.ExecutionID, m.Label, m.RunID).Observe(m.Latency)
	s.statusCounter.WithLabelValues(m.ExecutionID, m.ScenarioID, m.RunID, m.EngineID, m.Label, m.Status).Inc()
	s.threadsGauge.WithLabelValues(m.ExecutionID, m.ScenarioID, m.RunID, m.EngineID).Set(m.Threads)
}

// DeleteExecution drops every series carrying the collection's id label.
func (s *Sink) DeleteExecution(executionID int64) {
	id := strconv.FormatInt(executionID, 10)
	match := prometheus.Labels{"execution_id": id}
	s.collectionLatency.DeletePartialMatch(match)
	s.planLatency.DeletePartialMatch(match)
	s.labelLatency.DeletePartialMatch(match)
	s.statusCounter.DeletePartialMatch(match)
	s.threadsGauge.DeletePartialMatch(match)
}
