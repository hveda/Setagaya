package ports

import "github.com/heridotlife/Setagaya/internal/domain/engine"

// MetricsSink records engine measurements into a metrics backend (Prometheus in
// production). It is write-only: scraping/exposition is the backend's concern.
type MetricsSink interface {
	// Record ingests one engine measurement (latency, threads, status).
	Record(m engine.Metric)
	// DeleteCollection removes all series for a collection, called on purge so
	// stale label sets do not accumulate.
	DeleteCollection(collectionID int64)
}
