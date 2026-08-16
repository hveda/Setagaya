package report_test

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// The property the whole design rests on: a report accumulated batch by batch as
// a run happens must equal one built in a single pass from every interval. Two
// paths that summarised a run differently would eventually disagree about a
// verdict, and the one that disagreed would be the one nobody tested.
func TestAccumulator_MatchesBuild(t *testing.T) {
	t.Parallel()

	meta := report.Meta{
		ExecutionID: 7, ScenarioID: 11, RunID: 3,
		Engine:    taurus.ExecutorJMeter,
		StartedAt: time.Unix(1000, 0).UTC(),
		EndedAt:   time.Unix(1060, 0).UTC(),
		Outcome:   taurus.OutcomeFailed,
		Requested: report.Load{Concurrency: 40, Throughput: 200, DurationSeconds: 60},
	}
	intervals := syntheticRun(rand.New(rand.NewSource(7)))

	acc := report.NewAccumulator()
	for _, iv := range intervals {
		acc.Add(iv)
	}
	incremental := acc.Report(meta)

	oneShot := report.Build(report.Input{
		ExecutionID: meta.ExecutionID, ScenarioID: meta.ScenarioID, RunID: meta.RunID,
		Engine:    meta.Engine,
		StartedAt: meta.StartedAt, EndedAt: meta.EndedAt,
		Outcome:   meta.Outcome,
		Requested: meta.Requested,
		Intervals: intervals,
	})

	if !reflect.DeepEqual(incremental, oneShot) {
		t.Errorf("accumulated report differs from the one-pass build:\n got %+v\nwant %+v",
			incremental, oneShot)
	}
	// Guard against the comparison passing because both are empty.
	if incremental.Achieved.Samples == 0 || len(incremental.Errors) == 0 {
		t.Fatalf("the fixture produced nothing to compare: %+v", incremental)
	}
}

// A run outlives the process measuring it: the control plane restarts, or the
// next batch lands on another replica. Whatever was accumulated has to survive
// being written down and read back, or a run would produce one report per
// restart instead of one report.
func TestAccumulator_SurvivesSnapshotAndRestore(t *testing.T) {
	t.Parallel()

	meta := report.Meta{
		ExecutionID: 1, RunID: 2, Outcome: taurus.OutcomePassed,
		StartedAt: time.Unix(1000, 0).UTC(),
		EndedAt:   time.Unix(1060, 0).UTC(),
		Requested: report.Load{Concurrency: 40, DurationSeconds: 60},
	}
	intervals := syntheticRun(rand.New(rand.NewSource(11)))

	// Measure half the run, write it down, read it back, measure the rest.
	first := report.NewAccumulator()
	for _, iv := range intervals[:len(intervals)/2] {
		first.Add(iv)
	}
	resumed := report.Restore(first.Snapshot())
	for _, iv := range intervals[len(intervals)/2:] {
		resumed.Add(iv)
	}

	uninterrupted := report.NewAccumulator()
	for _, iv := range intervals {
		uninterrupted.Add(iv)
	}

	if !reflect.DeepEqual(resumed.Report(meta), uninterrupted.Report(meta)) {
		t.Errorf("a restart changed the report:\n got %+v\nwant %+v",
			resumed.Report(meta), uninterrupted.Report(meta))
	}
}

// Snapshots are written to a database and compared across restarts, so the same
// state must always produce the same bytes -- Go's map iteration order must not
// leak into it.
func TestAccumulator_SnapshotIsOrdered(t *testing.T) {
	t.Parallel()

	build := func() report.Snapshot {
		acc := report.NewAccumulator()
		for _, iv := range syntheticRun(rand.New(rand.NewSource(3))) {
			acc.Add(iv)
		}
		return acc.Snapshot()
	}
	if !reflect.DeepEqual(build(), build()) {
		t.Error("two snapshots of the same state differ")
	}

	s := build()
	if len(s.Labels) == 0 || len(s.Seconds) == 0 || len(s.Signatures) == 0 {
		t.Fatalf("the fixture snapshot is missing a dimension: %+v", s)
	}
	for i := 1; i < len(s.Labels); i++ {
		if s.Labels[i-1].Label >= s.Labels[i].Label {
			t.Errorf("labels are not ordered: %q then %q", s.Labels[i-1].Label, s.Labels[i].Label)
		}
	}
	for i := 1; i < len(s.Seconds); i++ {
		if s.Seconds[i-1].Second >= s.Seconds[i].Second {
			t.Errorf("seconds are not ordered: %d then %d", s.Seconds[i-1].Second, s.Seconds[i].Second)
		}
	}
}

// An empty accumulator is the state a run is in before its first pod reports.
// It has to summarise cleanly rather than dividing by zero.
func TestAccumulator_Empty(t *testing.T) {
	t.Parallel()

	rep := report.NewAccumulator().Report(report.Meta{
		ExecutionID: 1, RunID: 1, Outcome: taurus.OutcomeError,
		Requested: report.Load{Concurrency: 10, DurationSeconds: 60},
	})
	if rep.Achieved.Samples != 0 || rep.ErrorRate != 0 || rep.Achieved.Concurrency != 0 {
		t.Errorf("empty run = %+v", rep.Achieved)
	}
	if len(rep.Labels) != 0 || len(rep.Errors) != 0 {
		t.Errorf("empty run reported labels %+v and errors %+v", rep.Labels, rep.Errors)
	}
	if s := report.NewAccumulator().Snapshot(); len(s.Labels) != 0 || len(s.Seconds) != 0 {
		t.Errorf("empty snapshot = %+v", s)
	}
}

// syntheticRun is four shards over ten seconds across two labels, with the
// engine's own aggregate row, a mix of failures, and a slow tail only one shard
// sees -- enough shape that a difference between two summaries would show.
func syntheticRun(rng *rand.Rand) []metrics.Interval {
	labels := []string{"checkout-cart", "checkout-pay"}
	wordings := []metrics.ErrorGroup{
		{Message: "Not Found", ResponseCode: "404"},
		{Message: "Request to http://svc/cart didn't succeed (404)", ResponseCode: "404"},
		{Message: "Internal Server Error", ResponseCode: "500"},
		{Message: "socket: too many open files"},
		{Message: "something went sideways"},
	}

	var out []metrics.Interval
	for ts := int64(1000); ts < 1010; ts++ {
		for shard := range 4 {
			for _, label := range labels {
				iv := metrics.Interval{
					Timestamp:   ts,
					Label:       label,
					Concurrency: 5 + rng.Intn(3),
					Samples:     int64(10 + rng.Intn(20)),
					Latency:     metrics.Histogram{0.01: 90, 0.2: 9},
				}
				if shard == 0 {
					iv.Latency[2.5] = 1 // a tail only one shard sees
				}
				if rng.Intn(3) == 0 {
					e := wordings[rng.Intn(len(wordings))]
					e.Count = int64(1 + rng.Intn(4))
					iv.Failed = e.Count
					iv.Errors = []metrics.ErrorGroup{e}
				}
				iv.Succeeded = iv.Samples - iv.Failed
				out = append(out, iv)
			}
			out = append(out, metrics.Interval{
				Timestamp: ts, Label: report.TotalLabel, Concurrency: 12,
			})
		}
	}
	return out
}
