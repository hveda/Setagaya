package ports

import "github.com/heridotlife/Setagaya/internal/domain/engine"

// MetricsSink records engine measurements into a metrics backend (Prometheus in
// production). It is write-only: scraping/exposition is the backend's concern.
type MetricsSink interface {
	// Record ingests one engine measurement (latency, threads, status).
	Record(m engine.Metric)
	// DeleteExecution removes all series for an execution, called on purge so
	// stale label sets do not accumulate.
	DeleteExecution(executionID int64)
}
