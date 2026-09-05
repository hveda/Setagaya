package report

import "github.com/heridotlife/honryu/internal/domain/metrics"

// MergeSecond folds every row a run's pods reported for one second into the
// run-wide measurement of that second: what a per-second series is charted
// from, and what the series store persists one row of per second.
//
// Label rows merge additively -- counts summed, response-time histograms
// unioned -- which is what Interval.Merge does pairwise; this is the same
// arithmetic expressed over a whole second's rows at once. The engine's own
// aggregate row (TotalLabel) is kept out of those counts because its samples
// re-count the label rows beside it: the same rule Accumulator.Add applies,
// written here once so a second merged for a series cannot drift from a second
// the report itself counted.
//
// The total row's concurrency is returned separately rather than summed in: it
// is the engine's own reading of that pod's virtual users, counted between
// requests where per-label rows cannot see them. A second's VUs are the max of
// the two readings -- Accumulator.peak's rule, applied per second -- and the
// caller applies it, because only the caller knows whether it is holding one
// pod's rows (sum first, max after) or a run-wide row already.
//
// The returned Interval describes no label and no pod, so Label and Seq are
// left zero; Timestamp is left zero too, since the caller grouped by it to
// call this and already knows it. Latency is nil when no row carried buckets,
// so a second with no samples has no latency entries to speak of.
func MergeSecond(rows []metrics.Interval) (merged metrics.Interval, engineConcurrency int) {
	hist := metrics.Histogram{}
	for _, row := range rows {
		if row.Label == TotalLabel {
			engineConcurrency += row.Concurrency
			continue
		}
		merged.Concurrency += row.Concurrency
		merged.Samples += row.Samples
		merged.Succeeded += row.Succeeded
		merged.Failed += row.Failed
		merged.Bytes += row.Bytes
		hist.Merge(row.Latency)
	}
	if len(hist) > 0 {
		merged.Latency = hist
	}
	return merged, engineConcurrency
}
