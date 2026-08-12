package report

import (
	"time"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// TrendPoint is one run's contribution to its execution's trend: the metrics
// a reader watches over time as raw series, plus the one signal that is
// flagged -- whether the run regressed on hitting its target QPS relative to
// its nearest comparable predecessor.
type TrendPoint struct {
	RunID               int64
	StartedAt           time.Time
	Outcome             taurus.Outcome
	AchievedThroughput  float64
	RequestedThroughput float64
	ErrorRate           float64
	// P50/P90/P95/P99 are raw latency series -- advisory only, never
	// auto-flagged (see spec: noisy metrics are context, not a gate).
	P50, P90, P95, P99 float64
	// HitTargetQPS mirrors !Report.ShortOfRequest(): true when no target was
	// requested, or the run achieved at least 95% of it.
	HitTargetQPS bool
	// HasComparablePredecessor is false when no earlier report in the trend
	// is Comparable to this one -- "no baseline", never a false regression.
	HasComparablePredecessor bool
	// Regressed is true only when this run did NOT hit its target QPS while
	// its nearest comparable predecessor did. The reverse transition (missed
	// then hit) is an improvement, not a regression, and is not separately
	// flagged here -- a reader sees it directly in HitTargetQPS's own value
	// per point.
	Regressed bool
}

// Trend is an execution's run-over-run history, most-recent first -- the
// same order BuildTrend requires of its input.
type Trend struct {
	ExecutionID int64
	Points      []TrendPoint
}

// Comparable reports whether a and b are the same kind of run: same
// execution, requested load (concurrency, throughput, duration), engine, and
// cluster. Two runs that differ in any of these are not held to the same
// hit-target-QPS baseline -- comparing their outcome would compare apples to
// oranges.
func Comparable(a, b Report) bool {
	return a.ExecutionID == b.ExecutionID &&
		a.Engine == b.Engine &&
		a.Cluster == b.Cluster &&
		a.Requested.Concurrency == b.Requested.Concurrency &&
		a.Requested.Throughput == b.Requested.Throughput &&
		a.Requested.DurationSeconds == b.Requested.DurationSeconds
}

// BuildTrend builds an execution's trend from its reports, which must already
// be ordered most-recent first (as ListReports returns them). For each
// report, the nearest predecessor in the remainder of the list that is
// Comparable to it becomes its baseline -- not necessarily the very next
// entry, since an intervening reconfigured run (different engine, cluster, or
// requested load) has no comparable baseline of its own but must not block a
// later report from finding one further back.
func BuildTrend(reports []Report) Trend {
	var t Trend
	if len(reports) > 0 {
		t.ExecutionID = reports[0].ExecutionID
	}
	t.Points = make([]TrendPoint, len(reports))
	for i, r := range reports {
		p := TrendPoint{
			RunID: r.RunID, StartedAt: r.StartedAt, Outcome: r.Outcome,
			AchievedThroughput: r.Achieved.Throughput, RequestedThroughput: r.Requested.Throughput,
			ErrorRate:    r.ErrorRate,
			P50:          r.Latency[50],
			P90:          r.Latency[90],
			P95:          r.Latency[95],
			P99:          r.Latency[99],
			HitTargetQPS: !r.ShortOfRequest(),
		}
		for _, other := range reports[i+1:] {
			if Comparable(r, other) {
				p.HasComparablePredecessor = true
				p.Regressed = !p.HitTargetQPS && !other.ShortOfRequest()
				break
			}
		}
		t.Points[i] = p
	}
	return t
}
