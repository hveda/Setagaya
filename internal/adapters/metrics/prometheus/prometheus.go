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
			Namespace: "setagaya", Name: "latency_collection",
			Help: "Percentile latency of a collection", Objectives: objectives,
		}, []string{"collection_id", "run_id"}),
		planLatency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: "setagaya", Name: "latency_plan",
			Help: "Percentile latency of a plan", Objectives: objectives,
		}, []string{"collection_id", "plan_id", "run_id"}),
		labelLatency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: "setagaya", Name: "latency_label",
			Help: "Percentile latency of a label", Objectives: objectives,
		}, []string{"collection_id", "label", "run_id"}),
		statusCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "setagaya", Name: "status_counter",
			Help: "Count of responses grouped by response code",
		}, []string{"collection_id", "plan_id", "run_id", "engine_no", "label", "status"}),
		threadsGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "setagaya", Name: "threads_gauge",
			Help: "Current number of threads running in the engine",
		}, []string{"collection_id", "plan_id", "run_id", "engine_no"}),
	}
	reg.MustRegister(s.collectionLatency, s.planLatency, s.labelLatency, s.statusCounter, s.threadsGauge)
	return s
}

// Record ingests one measurement into the latency, status, and threads series.
func (s *Sink) Record(m engine.Metric) {
	s.collectionLatency.WithLabelValues(m.CollectionID, m.RunID).Observe(m.Latency)
	s.planLatency.WithLabelValues(m.CollectionID, m.PlanID, m.RunID).Observe(m.Latency)
	s.labelLatency.WithLabelValues(m.CollectionID, m.Label, m.RunID).Observe(m.Latency)
	s.statusCounter.WithLabelValues(m.CollectionID, m.PlanID, m.RunID, m.EngineID, m.Label, m.Status).Inc()
	s.threadsGauge.WithLabelValues(m.CollectionID, m.PlanID, m.RunID, m.EngineID).Set(m.Threads)
}

// DeleteCollection drops every series carrying the collection's id label.
func (s *Sink) DeleteCollection(collectionID int64) {
	id := strconv.FormatInt(collectionID, 10)
	match := prometheus.Labels{"collection_id": id}
	s.collectionLatency.DeletePartialMatch(match)
	s.planLatency.DeletePartialMatch(match)
	s.labelLatency.DeletePartialMatch(match)
	s.statusCounter.DeletePartialMatch(match)
	s.threadsGauge.DeletePartialMatch(match)
}
