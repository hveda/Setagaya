package report

import (
	"reflect"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

func TestMergeSecond(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		rows []metrics.Interval
		want metrics.Interval
		// wantEngine is the engine reading of the second's concurrency.
		wantEngine int
	}{
		{
			name: "no rows yield a zero second",
			rows: nil,
		},
		{
			name: "one label row passes its counts through",
			rows: []metrics.Interval{{
				Timestamp: 1000, Label: "cart", Concurrency: 5,
				Samples: 20, Succeeded: 18, Failed: 2, Bytes: 900,
				Latency: metrics.Histogram{0.01: 18, 0.2: 2},
			}},
			want: metrics.Interval{
				Concurrency: 5, Samples: 20, Succeeded: 18, Failed: 2, Bytes: 900,
				Latency: metrics.Histogram{0.01: 18, 0.2: 2},
			},
		},
		{
			name: "two pods' rows for the same second sum",
			rows: []metrics.Interval{
				{Timestamp: 1000, Label: "cart", Concurrency: 5, Samples: 20, Succeeded: 20, Bytes: 100, Latency: metrics.Histogram{0.01: 20}},
				{Timestamp: 1000, Label: "cart", Concurrency: 4, Samples: 12, Succeeded: 11, Failed: 1, Bytes: 60, Latency: metrics.Histogram{0.01: 11, 0.5: 1}},
			},
			want: metrics.Interval{
				Concurrency: 9, Samples: 32, Succeeded: 31, Failed: 1, Bytes: 160,
				Latency: metrics.Histogram{0.01: 31, 0.5: 1},
			},
		},
		{
			name: "the engine's own aggregate row counts concurrency only",
			rows: []metrics.Interval{
				{Timestamp: 1000, Label: "cart", Concurrency: 5, Samples: 20, Succeeded: 20, Bytes: 100, Latency: metrics.Histogram{0.01: 20}},
				{Timestamp: 1000, Label: TotalLabel, Concurrency: 7, Samples: 20, Succeeded: 20, Bytes: 100, Latency: metrics.Histogram{0.01: 20}},
			},
			want: metrics.Interval{
				Concurrency: 5, Samples: 20, Succeeded: 20, Bytes: 100,
				Latency: metrics.Histogram{0.01: 20},
			},
			wantEngine: 7,
		},
		{
			name: "several total rows' concurrency sums into the engine reading",
			rows: []metrics.Interval{
				{Timestamp: 1000, Label: TotalLabel, Concurrency: 7},
				{Timestamp: 1000, Label: TotalLabel, Concurrency: 3},
			},
			wantEngine: 10,
		},
		{
			name: "rows without buckets leave latency unset",
			rows: []metrics.Interval{
				{Timestamp: 1000, Label: "cart", Samples: 0, Concurrency: 2},
			},
			want: metrics.Interval{Concurrency: 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, engine := MergeSecond(tc.rows)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("merged = %+v, want %+v", got, tc.want)
			}
			if engine != tc.wantEngine {
				t.Errorf("engine concurrency = %d, want %d", engine, tc.wantEngine)
			}
		})
	}
}

// MergeSecond must never retain or mutate a caller's histogram: the caller's
// rows are reused across calls (a batch's intervals are folded into several
// accumulators), so aliasing would corrupt one second with another's buckets.
func TestMergeSecondDoesNotRetainCallerHistograms(t *testing.T) {
	t.Parallel()
	rows := []metrics.Interval{
		{Timestamp: 1000, Label: "cart", Samples: 1, Latency: metrics.Histogram{0.01: 1}},
		{Timestamp: 1000, Label: "pay", Samples: 1, Latency: metrics.Histogram{0.05: 1}},
	}
	first, _ := MergeSecond(rows)
	second, _ := MergeSecond(rows[1:]) // pay only, from the same backing array
	if _, ok := second.Latency[0.01]; ok {
		t.Fatalf("second merge retained cart's bucket: %+v", second.Latency)
	}
	if first.Latency.Count() != 2 {
		t.Fatalf("first merge = %+v, want both buckets", first.Latency)
	}
}
