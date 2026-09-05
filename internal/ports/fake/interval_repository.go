package fake

import (
	"context"
	"sort"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
)

// seriesSecond is one row of the per-second series a run builds up -- the same
// shape execution_report_series stores, so the fake's arithmetic mirrors the
// adapter's columns rather than inventing its own. The two concurrency
// readings stay separate because which one is a second's virtual users depends
// on what the engine sent (Accumulator.peak's rule), and the max is applied on
// the way out, at read time, exactly as the adapter applies GREATEST.
type seriesSecond struct {
	engine int
	labels int

	samples int64
	failed  int64
	bytes   int64
	latency metrics.Histogram
}

// ListIntervalsByRun returns the run's per-second series, ascending by second.
//
// The fake for ports.IntervalRepository is the same object that fakes
// ports.ReportProgress, because in the real system they share one transaction
// and one truth: Absorb writes the series, and separating the fakes would let
// a test seed a series no absorb path could have produced.
func (p *ReportProgress) ListIntervalsByRun(_ context.Context, runID int64) ([]metrics.Interval, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	seconds := p.series[runID]
	out := make([]metrics.Interval, 0, len(seconds))
	for ts := range seconds {
		row := seconds[ts]
		iv := metrics.Interval{
			Timestamp: ts, Samples: row.samples, Failed: row.failed, Bytes: row.bytes,
		}
		if row.engine > row.labels {
			iv.Concurrency = row.engine
		} else {
			iv.Concurrency = row.labels
		}
		if len(row.latency) > 0 {
			iv.Latency = row.latency
		}
		out = append(out, iv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out, nil
}

// absorbSeries extends the run's permanent series with a batch's fresh
// intervals -- those past the shard's watermark, which Absorb has already
// selected. Merging is report.MergeSecond's, so the fake and the adapter apply
// the same domain rule to the same rows; only the additive mechanics differ
// (maps here, locked SQL upserts there).
//
// The series deliberately outlives Discard: it is the report's evidence, not
// working state, which is what the interval contract's survives-finalisation
// case pins for real adapters too.
func (p *ReportProgress) absorbSeries(runID int64, fresh []metrics.Interval) {
	bySecond := make(map[int64][]metrics.Interval, len(fresh))
	for _, iv := range fresh {
		bySecond[iv.Timestamp] = append(bySecond[iv.Timestamp], iv)
	}
	seconds := p.series[runID]
	if seconds == nil {
		seconds = map[int64]*seriesSecond{}
		p.series[runID] = seconds
	}
	for ts, rows := range bySecond {
		merged, engine := report.MergeSecond(rows)
		row, ok := seconds[ts]
		if !ok {
			row = &seriesSecond{}
			seconds[ts] = row
		}
		row.engine += engine
		row.labels += merged.Concurrency
		row.samples += merged.Samples
		row.failed += merged.Failed
		row.bytes += merged.Bytes
		if len(merged.Latency) > 0 {
			if row.latency == nil {
				row.latency = metrics.Histogram{}
			}
			row.latency.Merge(merged.Latency)
		}
	}
}
