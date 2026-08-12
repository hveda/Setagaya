package report_test

import (
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

func baseReport(runID int64, startSeconds int) report.Report {
	return report.Report{
		ExecutionID: 1, RunID: runID, Engine: taurus.ExecutorJMeter, Cluster: "prod-eu",
		StartedAt: time.Unix(int64(startSeconds), 0).UTC(),
		Outcome:   taurus.OutcomePassed,
		Requested: report.Load{Concurrency: 10, Throughput: 100, DurationSeconds: 60},
		Achieved:  report.Load{Throughput: 100},
	}
}

func TestComparable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutateB func(report.Report) report.Report
		want    bool
	}{
		{"identical", func(b report.Report) report.Report { return b }, true},
		{"different execution", func(b report.Report) report.Report { b.ExecutionID = 2; return b }, false},
		{"different engine", func(b report.Report) report.Report { b.Engine = taurus.ExecutorK6; return b }, false},
		{"different cluster", func(b report.Report) report.Report { b.Cluster = "prod-us"; return b }, false},
		{"different concurrency", func(b report.Report) report.Report { b.Requested.Concurrency = 20; return b }, false},
		{"different throughput", func(b report.Report) report.Report { b.Requested.Throughput = 200; return b }, false},
		{"different duration", func(b report.Report) report.Report { b.Requested.DurationSeconds = 0; return b }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := baseReport(1, 0)
			b := tc.mutateB(baseReport(2, 100))
			if got := report.Comparable(a, b); got != tc.want {
				t.Fatalf("Comparable(a, b) = %v, want %v", got, tc.want)
			}
			// Comparable is symmetric.
			if got := report.Comparable(b, a); got != tc.want {
				t.Fatalf("Comparable(b, a) = %v, want %v (symmetry)", got, tc.want)
			}
		})
	}
}

func TestBuildTrend_Empty(t *testing.T) {
	t.Parallel()
	got := report.BuildTrend(nil)
	if got.ExecutionID != 0 || len(got.Points) != 0 {
		t.Fatalf("BuildTrend(nil) = %+v, want empty", got)
	}
}

func TestBuildTrend_SingleReport_NoComparablePredecessor(t *testing.T) {
	t.Parallel()
	got := report.BuildTrend([]report.Report{baseReport(1, 100)})
	if len(got.Points) != 1 {
		t.Fatalf("BuildTrend = %+v, want 1 point", got)
	}
	p := got.Points[0]
	if p.HasComparablePredecessor || p.Regressed {
		t.Fatalf("point = %+v, want no comparable predecessor and not regressed", p)
	}
}

// Most-recent-first: reports[0] is newer than reports[1]. reports[0] missed
// its target while reports[1] hit it -- a real regression.
func TestBuildTrend_FlipToMissedTarget_FlagsRegressed(t *testing.T) {
	t.Parallel()
	newer := baseReport(2, 200)
	newer.Achieved.Throughput = 50 // 50% of 100 requested: short of target
	older := baseReport(1, 100)
	older.Achieved.Throughput = 100 // hit target

	got := report.BuildTrend([]report.Report{newer, older})
	if len(got.Points) != 2 {
		t.Fatalf("BuildTrend = %+v, want 2 points", got)
	}
	if got.Points[0].HitTargetQPS {
		t.Fatalf("newer point = %+v, want HitTargetQPS false", got.Points[0])
	}
	if !got.Points[0].HasComparablePredecessor || !got.Points[0].Regressed {
		t.Fatalf("newer point = %+v, want a comparable predecessor and Regressed true", got.Points[0])
	}
	// The older point has no predecessor of its own in this two-report trend.
	if got.Points[1].HasComparablePredecessor || got.Points[1].Regressed {
		t.Fatalf("older point = %+v, want no comparable predecessor", got.Points[1])
	}
}

// The reverse transition (missed, then hit) is an improvement, not a
// regression -- Regressed stays false even though the signal changed.
func TestBuildTrend_FlipToHitTarget_NotFlaggedRegressed(t *testing.T) {
	t.Parallel()
	newer := baseReport(2, 200)
	newer.Achieved.Throughput = 100 // now hits target
	older := baseReport(1, 100)
	older.Achieved.Throughput = 50 // previously missed

	got := report.BuildTrend([]report.Report{newer, older})
	if !got.Points[0].HitTargetQPS {
		t.Fatalf("newer point = %+v, want HitTargetQPS true", got.Points[0])
	}
	if !got.Points[0].HasComparablePredecessor {
		t.Fatalf("newer point = %+v, want a comparable predecessor", got.Points[0])
	}
	if got.Points[0].Regressed {
		t.Fatalf("newer point = %+v, want Regressed false (improvement, not a regression)", got.Points[0])
	}
}

// A predecessor that isn't comparable (different engine) is skipped in favor
// of an earlier one that is -- an intervening reconfigured run does not
// erase the trend's history.
func TestBuildTrend_SkipsNonComparablePredecessorForAnEarlierOne(t *testing.T) {
	t.Parallel()
	newest := baseReport(3, 300)
	newest.Achieved.Throughput = 50 // short of target
	middle := baseReport(2, 200)    // different engine: not comparable to newest
	middle.Engine = taurus.ExecutorK6
	middle.Achieved.Throughput = 100
	oldest := baseReport(1, 100) // same engine/cluster/load as newest: comparable
	oldest.Achieved.Throughput = 100

	got := report.BuildTrend([]report.Report{newest, middle, oldest})
	p := got.Points[0]
	if !p.HasComparablePredecessor {
		t.Fatalf("newest point = %+v, want a comparable predecessor found past the non-comparable middle report", p)
	}
	if !p.Regressed {
		t.Fatalf("newest point = %+v, want Regressed true (oldest hit target, newest didn't)", p)
	}
}

// Two consecutive comparable runs, both hitting target: no regression, but a
// baseline exists (HasComparablePredecessor true).
func TestBuildTrend_BothHitTarget_NoRegression(t *testing.T) {
	t.Parallel()
	newer := baseReport(2, 200)
	older := baseReport(1, 100)
	got := report.BuildTrend([]report.Report{newer, older})
	if !got.Points[0].HasComparablePredecessor || got.Points[0].Regressed {
		t.Fatalf("point = %+v, want a comparable predecessor and Regressed false", got.Points[0])
	}
}

// A run that requested no target QPS is always HitTargetQPS true (the
// no-target guard), so it can never register as regressed no matter how
// little it achieved.
func TestBuildTrend_NoTargetRequested_NeverRegresses(t *testing.T) {
	t.Parallel()
	newer := baseReport(2, 200)
	newer.Requested.Throughput = 0
	newer.Achieved.Throughput = 1
	older := baseReport(1, 100)
	older.Requested.Throughput = 0
	older.Achieved.Throughput = 1000

	got := report.BuildTrend([]report.Report{newer, older})
	if !got.Points[0].HitTargetQPS {
		t.Fatalf("point = %+v, want HitTargetQPS true (no target requested)", got.Points[0])
	}
	if got.Points[0].Regressed {
		t.Fatalf("point = %+v, want Regressed false", got.Points[0])
	}
}

func TestBuildTrend_CarriesRawSeriesFields(t *testing.T) {
	t.Parallel()
	r := baseReport(1, 100)
	r.ErrorRate = 0.05
	r.Latency = report.Percentiles{50: 0.01, 90: 0.05, 95: 0.08, 99: 0.2}
	got := report.BuildTrend([]report.Report{r})
	p := got.Points[0]
	if p.ErrorRate != 0.05 || p.P50 != 0.01 || p.P90 != 0.05 || p.P95 != 0.08 || p.P99 != 0.2 {
		t.Fatalf("point = %+v, want the raw latency/error-rate series carried through", p)
	}
	if p.RunID != 1 || p.Outcome != taurus.OutcomePassed {
		t.Fatalf("point = %+v, want RunID/Outcome carried through", p)
	}
	if got.ExecutionID != 1 {
		t.Fatalf("Trend.ExecutionID = %d, want 1", got.ExecutionID)
	}
}
