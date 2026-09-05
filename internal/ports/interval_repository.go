package ports

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// IntervalRepository reads the per-second measurements a run produced, merged
// across pods and labels -- the run's series, as distinct from the verdicts
// its report carries.
//
// Rows are written as a run's shards push, inside ReportProgress.Absorb's own
// transaction: the shard sequence watermark has already filtered a retry's
// re-sent intervals before a row is written, so what this returns counts each
// pod-second once. The rows outlive the working state -- the shape of a run is
// part of the evidence its report is judged on -- which is why reading them is
// a capability of its own rather than a ReportProgress method.
type IntervalRepository interface {
	// ListIntervalsByRun returns a run's per-second measurements ordered by
	// timestamp. Each interval is one second of the run as a whole: Label is
	// empty, Seq is zero (identity was per shard, and is gone once merged),
	// Concurrency is the second's virtual users -- the larger of the engine's
	// and the per-label rows' readings -- and Latency the second's merged
	// response-time buckets, unset when the second recorded no samples.
	//
	// A run with no stored seconds returns an empty slice, not an error: runs
	// that predate the series store are simply without a shape to chart, and
	// an empty report already tells the caller the run exists.
	ListIntervalsByRun(ctx context.Context, runID int64) ([]metrics.Interval, error)
}
