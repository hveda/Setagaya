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

// DeleteCollection records the deletion.
func (s *MetricsSink) DeleteCollection(collectionID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, collectionID)
}

// Recorded returns a copy of the recorded metrics.
func (s *MetricsSink) Recorded() []engine.Metric {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]engine.Metric(nil), s.recorded...)
}

// Deleted returns the collection ids passed to DeleteCollection.
func (s *MetricsSink) Deleted() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.deleted...)
}
