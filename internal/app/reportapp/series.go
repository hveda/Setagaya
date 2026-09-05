// Package reportapp is the report use-case: what a reader asks of a run's
// report after the run is over. Its first job is the per-second series a
// results page charts -- the shape of the run, as distinct from the verdicts
// the report itself carries.
package reportapp

import (
	"sort"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
)

// SeriesPoint is one second of a run, as a chart reads it.
//
// Latency maps each requested percentile to the response time at it, in
// seconds (the domain convention); it is left unset on seconds that recorded
// no samples, which is what "no latency entries for that second" means to a
// chart. RPS is samples per second and ErrPct the failure share in percent.
type SeriesPoint struct {
	Ts      int64              `json:"ts"`
	VUs     float64            `json:"vus"`
	RPS     float64            `json:"rps"`
	ErrPct  float64            `json:"err_pct"`
	Latency report.Percentiles `json:"latency,omitempty"`
}

// seriesPercentiles are the percentiles the series endpoint serves: the same
// set every report already carries (report.reportedPercentiles), so a chart
// and the percentile table beneath it can never disagree about which lines
// exist.
var seriesPercentiles = []float64{50, 90, 95, 99}

// SeriesPercentiles returns a copy of the percentiles the series endpoint
// serves, for callers that need the list itself.
func SeriesPercentiles() []float64 {
	out := make([]float64, len(seriesPercentiles))
	copy(out, seriesPercentiles)
	return out
}

// duplicateKey identifies a re-sent interval: the second it covers, the label
// it measured, and the sequence the sidecar's stream gave it.
type duplicateKey struct {
	ts    int64
	label string
	seq   int64
}

// BuildSeries folds a run's intervals into one point per measured second,
// ascending by timestamp, computing each requested percentile from the
// second's merged response-time buckets.
//
// Rows are grouped by timestamp and merged with report.MergeSecond, which sums
// counts across labels and unions histograms; a second's VUs are the max of
// the per-label and engine readings of its concurrency (Accumulator.peak's
// rule, per second).
//
// Duplicate handling -- the dedup key, and why it is not "highest seq wins".
// Intervals arrive from sidecar pushes, and a push whose response was lost is
// followed by a superset batch re-sending the intervals it still holds, each
// carrying its original sequence (interval.go). A duplicate is therefore a row
// repeating an exact (ts, label, seq) triple, and only exact repeats are
// dropped. A *higher* seq on the same (ts, label) is a different measurement,
// not a newer copy of one: another row of that label in that second, or
// another pod's, both of which must be summed rather than collapsed. The
// plan's first sketch -- dedupe by (shard, seq), keeping the highest seq --
// cannot be honoured from []metrics.Interval alone: the shard and its stream
// live on the Batch envelope, not the interval, and interval sequences count
// from one per stream, so at run start every pod's first interval shares
// (ts, label, seq=1) while being three distinct measurements (the progress
// contract pins this as "three shards at seq 1 are three intervals"). Summing
// across pods is Absorb's job, where the shard and its watermark are known;
// rows read back through IntervalRepository arrive already pod-merged, one
// per second, and rows with no sequence (Seq <= 0 -- storage-rebuilt rows)
// are never treated as duplicates.
func BuildSeries(intervals []metrics.Interval, pcts []float64) []SeriesPoint {
	if len(intervals) == 0 {
		return nil
	}

	seen := make(map[duplicateKey]struct{}, len(intervals))
	bySecond := make(map[int64][]metrics.Interval, len(intervals))
	for _, iv := range intervals {
		if iv.Seq > 0 {
			key := duplicateKey{ts: iv.Timestamp, label: iv.Label, seq: iv.Seq}
			if _, dup := seen[key]; dup {
				continue // a re-sent interval, not a new measurement
			}
			seen[key] = struct{}{}
		}
		bySecond[iv.Timestamp] = append(bySecond[iv.Timestamp], iv)
	}

	seconds := make([]int64, 0, len(bySecond))
	for ts := range bySecond {
		seconds = append(seconds, ts)
	}
	sort.Slice(seconds, func(i, j int) bool { return seconds[i] < seconds[j] })

	points := make([]SeriesPoint, 0, len(seconds))
	for _, ts := range seconds {
		merged, engine := report.MergeSecond(bySecond[ts])
		point := SeriesPoint{
			Ts:  ts,
			VUs: vus(merged.Concurrency, engine),
			// Each interval covers exactly one second, so the second's sample
			// count is its own rate.
			RPS: float64(merged.Samples),
		}
		if merged.Samples > 0 {
			point.ErrPct = float64(merged.Failed) / float64(merged.Samples) * 100
		}
		if merged.Latency.Count() > 0 {
			point.Latency = report.Percentiles(merged.Latency.Percentiles(pcts...))
		}
		points = append(points, point)
	}
	return points
}

// vus is a second's virtual users: the engine's own reading where it reported
// one, else the sum of the per-label rows -- the larger of the two, so a run
// is never charted as smaller than it demonstrably was.
func vus(labels, engine int) float64 {
	if engine > labels {
		return float64(engine)
	}
	return float64(labels)
}
