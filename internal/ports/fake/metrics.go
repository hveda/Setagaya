package fake

import (
	"sync"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// MetricsSink is an in-memory ports.MetricsSink that records calls for
// assertions in use-case tests.
type MetricsSink struct {
	mu       sync.Mutex
	recorded []engine.Metric
	deleted  []int64
}

// NewMetricsSink returns an empty sink.
func NewMetricsSink() *MetricsSink { return &MetricsSink{} }

var _ ports.MetricsSink = (*MetricsSink)(nil)

// Record appends the metric.
func (s *MetricsSink) Record(m engine.Metric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded = append(s.recorded, m)
}

// DeleteExecution records the deletion.
func (s *MetricsSink) DeleteExecution(executionID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, executionID)
}

// Recorded returns a copy of the recorded metrics.
func (s *MetricsSink) Recorded() []engine.Metric {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]engine.Metric(nil), s.recorded...)
}

// Deleted returns the execution ids passed to DeleteExecution.
func (s *MetricsSink) Deleted() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.deleted...)
}
