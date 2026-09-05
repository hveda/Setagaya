package reportapp_test

import (
	"reflect"
	"testing"

	"github.com/heridotlife/honryu/internal/app/reportapp"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
)

var chartPcts = []float64{50, 90, 95, 99}

// podRow is one pod's row for a second: sequenced, labelled, with buckets
// sized to its samples by default so latency fixtures stay honest.
func podRow(seq, ts int64, label string, concurrency int, samples, failed int64, hist metrics.Histogram) metrics.Interval {
	return metrics.Interval{
		Seq: seq, Timestamp: ts, Label: label, Concurrency: concurrency,
		Samples: samples, Succeeded: samples - failed, Failed: failed,
		Latency: hist,
	}
}

func TestBuildSeries(t *testing.T) {
	t.Parallel()

	t.Run("empty input yields no points", func(t *testing.T) {
		t.Parallel()
		for _, in := range [][]metrics.Interval{nil, {}} {
			if got := reportapp.BuildSeries(in, chartPcts); len(got) != 0 {
				t.Errorf("BuildSeries(%v) = %+v, want no points", in, got)
			}
		}
	})

	t.Run("a single pod's second across labels and the engine total", func(t *testing.T) {
		t.Parallel()
		// One pod's second: two label rows plus bzt's own __total__ row, whose
		// samples re-count the labels but whose concurrency is the pod's true
		// reading -- here higher than the labels' sum, as it can be when users
		// sit between requests.
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(1, 1000, "cart", 5, 20, 2, metrics.Histogram{0.01: 18, 0.2: 2}),
			podRow(2, 1000, "pay", 3, 10, 0, metrics.Histogram{0.01: 9, 0.05: 1}),
			podRow(3, 1000, report.TotalLabel, 9, 30, 2, metrics.Histogram{0.01: 27, 0.05: 1, 0.2: 2}),
		}, chartPcts)

		want := []reportapp.SeriesPoint{{
			Ts: 1000, VUs: 9, RPS: 30, ErrPct: float64(2) / float64(30) * 100,
			Latency: report.Percentiles{50: 0.01, 90: 0.01, 95: 0.2, 99: 0.2},
		}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("series = %+v, want %+v", got, want)
		}
	})

	t.Run("a duplicate superset re-send produces the identical series", func(t *testing.T) {
		t.Parallel()
		clean := []metrics.Interval{
			podRow(1, 1000, "cart", 5, 20, 1, metrics.Histogram{0.01: 19, 0.2: 1}),
			podRow(2, 1000, "pay", 3, 10, 0, metrics.Histogram{0.01: 10}),
			podRow(3, 1001, "cart", 5, 21, 0, metrics.Histogram{0.01: 21}),
		}
		// The sidecar never saw the first push's acknowledgement, so it
		// re-sends those intervals around the new one -- several times over,
		// and in no particular order.
		dirty := []metrics.Interval{
			clean[0], clean[1],
			clean[0], clean[2], clean[1],
			clean[2], clean[0],
		}
		if got, want := reportapp.BuildSeries(dirty, chartPcts), reportapp.BuildSeries(clean, chartPcts); !reflect.DeepEqual(got, want) {
			t.Errorf("duplicate-superset batch changed the series:\n got %+v\nwant %+v", got, want)
		}
	})

	t.Run("another pod at the same second and label is summed, not deduped", func(t *testing.T) {
		t.Parallel()
		// Two pods, the same label, the same second. Their sequences are each
		// stream's own count and share values freely; collapsing by highest
		// seq here would silently drop a pod's load.
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(1, 1000, "probe", 2, 5, 0, metrics.Histogram{0.01: 5}),
			podRow(5, 1000, "probe", 3, 7, 1, metrics.Histogram{0.01: 6, 0.5: 1}),
		}, chartPcts)
		want := []reportapp.SeriesPoint{{
			Ts: 1000, VUs: 5, RPS: 12, ErrPct: float64(1) / float64(12) * 100,
			Latency: report.Percentiles{50: 0.01, 90: 0.01, 95: 0.5, 99: 0.5},
		}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("series = %+v, want %+v (both pods summed)", got, want)
		}
	})

	t.Run("unsequenced rows are never duplicates", func(t *testing.T) {
		t.Parallel()
		// Storage-rebuilt per-second rows carry no sequence; two of them for
		// one second is malformed input, but merging -- not dropping -- is the
		// honest reading, because Seq 0 is the storage form, not a re-send.
		got := reportapp.BuildSeries([]metrics.Interval{
			{Timestamp: 1000, Samples: 1, Concurrency: 1},
			{Timestamp: 1000, Samples: 1, Concurrency: 1},
		}, chartPcts)
		if len(got) != 1 || got[0].RPS != 2 || got[0].VUs != 2 {
			t.Errorf("series = %+v, want one summed point (RPS 2, VUs 2)", got)
		}
	})

	t.Run("single-pod percentiles equal the direct computation", func(t *testing.T) {
		t.Parallel()
		hist := metrics.Histogram{0.01: 7, 0.05: 2, 0.2: 1}
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(1, 1000, "cart", 4, 10, 0, hist),
		}, chartPcts)
		if len(got) != 1 {
			t.Fatalf("series = %+v, want one point", got)
		}
		direct := report.Percentiles(hist.Percentiles(chartPcts...))
		if !reflect.DeepEqual(got[0].Latency, direct) {
			t.Errorf("latency = %v, want the histogram's own percentiles %v", got[0].Latency, direct)
		}
		// Spot-check two by hand so a shared bug in Percentiles cannot
		// certify itself: 10 samples, nearest-rank.
		if got[0].Latency[50] != 0.01 || got[0].Latency[95] != 0.2 {
			t.Errorf("p50 = %v, p95 = %v, want 0.01 and 0.2", got[0].Latency[50], got[0].Latency[95])
		}
	})

	t.Run("seconds with no samples carry no latency entries", func(t *testing.T) {
		t.Parallel()
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(1, 1000, "cart", 4, 0, 0, nil),
		}, chartPcts)
		want := []reportapp.SeriesPoint{{Ts: 1000, VUs: 4}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("series = %+v, want %+v (no RPS, no err pct, no latency)", got, want)
		}
	})

	t.Run("an engine-only second reports virtual users without samples", func(t *testing.T) {
		t.Parallel()
		// Some engines send the __total__ row before any request completes.
		// The chart must still see the users that second held.
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(1, 1000, report.TotalLabel, 6, 0, 0, nil),
		}, chartPcts)
		want := []reportapp.SeriesPoint{{Ts: 1000, VUs: 6}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("series = %+v, want %+v", got, want)
		}
	})

	t.Run("points ascend by timestamp whatever the input order", func(t *testing.T) {
		t.Parallel()
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(3, 1002, "cart", 1, 3, 0, metrics.Histogram{0.01: 3}),
			podRow(1, 1000, "cart", 1, 1, 0, metrics.Histogram{0.01: 1}),
			podRow(2, 1001, "cart", 1, 2, 0, metrics.Histogram{0.01: 2}),
		}, chartPcts)
		if len(got) != 3 || got[0].Ts != 1000 || got[1].Ts != 1001 || got[2].Ts != 1002 {
			t.Errorf("series timestamps = %+v, want ascending 1000, 1001, 1002", got)
		}
	})

	t.Run("only the requested percentiles are keys", func(t *testing.T) {
		t.Parallel()
		got := reportapp.BuildSeries([]metrics.Interval{
			podRow(1, 1000, "cart", 1, 4, 0, metrics.Histogram{0.01: 4}),
		}, []float64{50})
		if len(got[0].Latency) != 1 {
			t.Errorf("latency = %v, want exactly the requested p50", got[0].Latency)
		}
		if _, ok := got[0].Latency[50]; !ok {
			t.Errorf("latency = %v, want a p50 entry", got[0].Latency)
		}
	})
}

// The served percentile set is the report's own, so the chart and the
// percentile table beneath it show the same lines.
func TestSeriesPercentiles(t *testing.T) {
	t.Parallel()
	got := reportapp.SeriesPercentiles()
	want := []float64{50, 90, 95, 99}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SeriesPercentiles() = %v, want %v", got, want)
	}
	// A copy, not the package's own slice: a caller filtering or appending
	// must not change what the next caller gets.
	got[0] = 0
	if next := reportapp.SeriesPercentiles(); !reflect.DeepEqual(next, want) {
		t.Errorf("SeriesPercentiles() mutated to %v after a caller edited its copy", next)
	}
}
