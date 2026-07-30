package metrics_test

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// referencePercentile computes a percentile the obvious way: expand every
// bucket back into individual samples, sort them, and index. It shares the
// nearest-rank definition with the implementation but not the code path -- it
// never merges, never accumulates, and never sees a bucket -- so agreement
// between the two is evidence the merge and accumulation are right.
func referencePercentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func expand(h metrics.Histogram) []float64 {
	var out []float64
	for rt, n := range h {
		for i := int64(0); i < n; i++ {
			out = append(out, rt)
		}
	}
	return out
}

func TestHistogram_PercentileMatchesRawSamples(t *testing.T) {
	t.Parallel()

	h := metrics.Histogram{0.001: 10, 0.005: 30, 0.01: 40, 0.05: 15, 0.5: 5}
	raw := expand(h)

	for _, p := range []float64{0, 50, 90, 95, 99, 99.9, 100} {
		got := h.Percentile(p)
		want := referencePercentile(raw, p)
		if got != want {
			t.Errorf("p%.1f = %v, want %v", p, got, want)
		}
	}
}

// The claim sharding rests on: merging the shards' histograms and computing a
// percentile gives the same answer as computing it over every sample the shards
// produced between them. If this is wrong, every sharded run reports latency
// that nothing actually measured.
func TestHistogram_MergeEqualsCombinedSamples(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(1)) //#nosec G404 -- deterministic test data

	for trial := 0; trial < 200; trial++ {
		shards := rng.Intn(8) + 1
		merged := metrics.Histogram{}
		var all []float64

		for s := 0; s < shards; s++ {
			h := metrics.Histogram{}
			for i := 0; i < rng.Intn(50)+1; i++ {
				// Response times as bzt reports them: milliseconds, three decimals.
				rt := math.Round(rng.Float64()*1000) / 1000
				n := int64(rng.Intn(20) + 1)
				h[rt] += n
			}
			merged.Merge(h)
			all = append(all, expand(h)...)
		}

		if merged.Count() != int64(len(all)) {
			t.Fatalf("trial %d: merged count %d, want %d", trial, merged.Count(), len(all))
		}
		for _, p := range []float64{50, 90, 95, 99, 100} {
			got := merged.Percentile(p)
			want := referencePercentile(all, p)
			if got != want {
				t.Fatalf("trial %d (%d shards): p%.0f = %v, want %v", trial, shards, p, got, want)
			}
		}
	}
}

// The Phase 0 finding, encoded so it cannot come back: averaging each interval's
// own percentile under-reports the true one. It matters because the error runs
// in the dangerous direction -- latency looks better than it was, so a sale that
// should fail its criteria passes.
func TestHistogram_AveragingPercentilesUnderReports(t *testing.T) {
	t.Parallel()

	// Three intervals. Two are quick, one is slow and carries the tail.
	intervals := []metrics.Histogram{
		{0.010: 100},
		{0.010: 100},
		{0.010: 50, 2.000: 50},
	}

	merged := metrics.Histogram{}
	var perInterval []float64
	for _, h := range intervals {
		merged.Merge(h)
		perInterval = append(perInterval, h.Percentile(95))
	}

	truth := merged.Percentile(95)

	var sum float64
	for _, v := range perInterval {
		sum += v
	}
	averaged := sum / float64(len(perInterval))

	if averaged >= truth {
		t.Fatalf("averaged p95 %.3f should under-report the true %.3f", averaged, truth)
	}
	t.Logf("true p95 = %.3fs, average of interval p95s = %.3fs (%.0f%% under)",
		truth, averaged, (1-averaged/truth)*100)
}

func TestHistogram_MergeIsAdditiveAndSafe(t *testing.T) {
	t.Parallel()

	h := metrics.Histogram{0.01: 5}
	h.Merge(metrics.Histogram{0.01: 3, 0.02: 1})
	if h[0.01] != 8 || h[0.02] != 1 {
		t.Errorf("merge did not add counts: %+v", h)
	}

	// Merging nothing changes nothing -- a shard that reported no samples in an
	// interval must not disturb the aggregate.
	before := h.Count()
	h.Merge(nil)
	h.Merge(metrics.Histogram{})
	if h.Count() != before {
		t.Errorf("empty merge changed the count from %d to %d", before, h.Count())
	}

	// The source must not be aliased: mutating it afterwards cannot change the
	// merged result, or a sidecar reusing its buffer would corrupt the aggregate.
	src := metrics.Histogram{0.03: 2}
	h.Merge(src)
	src[0.03] = 999
	if h[0.03] != 2 {
		t.Errorf("merge aliased its source: %v", h[0.03])
	}
}

func TestHistogram_EmptyIsZero(t *testing.T) {
	t.Parallel()

	var h metrics.Histogram
	if h.Count() != 0 {
		t.Errorf("Count() = %d, want 0", h.Count())
	}
	if got := h.Percentile(95); got != 0 {
		t.Errorf("Percentile(95) = %v, want 0", got)
	}
	empty := metrics.Histogram{}
	if got := empty.Percentile(50); got != 0 {
		t.Errorf("empty Percentile(50) = %v, want 0", got)
	}
}

func TestHistogram_PercentilesAreMonotonic(t *testing.T) {
	t.Parallel()

	h := metrics.Histogram{0.001: 500, 0.01: 300, 0.1: 150, 1: 49, 10: 1}
	var last float64
	for p := 1.0; p <= 100; p++ {
		got := h.Percentile(p)
		if got < last {
			t.Fatalf("p%.0f = %v, lower than p%.0f = %v", p, got, p-1, last)
		}
		last = got
	}
}

// Percentile arguments come from configuration and reports, so out-of-range
// values must clamp rather than panic or index out of bounds.
func TestHistogram_PercentileClampsRange(t *testing.T) {
	t.Parallel()

	h := metrics.Histogram{0.01: 1, 0.02: 1, 0.03: 1}
	if got := h.Percentile(-10); got != 0.01 {
		t.Errorf("Percentile(-10) = %v, want the minimum 0.01", got)
	}
	if got := h.Percentile(1000); got != 0.03 {
		t.Errorf("Percentile(1000) = %v, want the maximum 0.03", got)
	}
}

// Percentiles computes several in one pass for a report. It must agree exactly
// with the single-value path, or a report would disagree with the criteria that
// were evaluated against the same run.
func TestHistogram_PercentilesAgreeWithPercentile(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(7)) //#nosec G404 -- deterministic test data
	want := []float64{50, 90, 95, 99, 99.9, 100}

	for trial := 0; trial < 100; trial++ {
		h := metrics.Histogram{}
		for i := 0; i < rng.Intn(60)+1; i++ {
			h[math.Round(rng.Float64()*1000)/1000] += int64(rng.Intn(30) + 1)
		}
		got := h.Percentiles(want...)
		if len(got) != len(want) {
			t.Fatalf("trial %d: got %d percentiles, want %d", trial, len(got), len(want))
		}
		for _, p := range want {
			if got[p] != h.Percentile(p) {
				t.Fatalf("trial %d: Percentiles()[%v] = %v, Percentile(%v) = %v",
					trial, p, got[p], p, h.Percentile(p))
			}
		}
	}
}

func TestHistogram_PercentilesHandlesEmptyAndUnsortedInput(t *testing.T) {
	t.Parallel()

	empty := metrics.Histogram{}
	got := empty.Percentiles(95, 50)
	if got[95] != 0 || got[50] != 0 {
		t.Errorf("empty histogram percentiles = %v, want zeroes", got)
	}

	// Callers pass whatever order they like; the answer must not depend on it.
	h := metrics.Histogram{0.01: 1, 0.02: 1, 0.03: 1, 0.04: 1}
	unsorted := h.Percentiles(99, 50, 100, 75)
	for _, p := range []float64{50, 75, 99, 100} {
		if unsorted[p] != h.Percentile(p) {
			t.Errorf("p%v = %v, want %v", p, unsorted[p], h.Percentile(p))
		}
	}

	if none := h.Percentiles(); len(none) != 0 {
		t.Errorf("Percentiles() with no arguments = %v, want empty", none)
	}
}
