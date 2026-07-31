// Package metrics holds the pure aggregation domain: combining what several
// engine pods measured into what the run as a whole did.
//
// Pure domain: arithmetic only, no I/O.
package metrics

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Histogram counts response times, keyed by the response time in seconds.
//
// It mirrors what bzt emits: its response times live in an HdrHistogram whose
// JSON form is exactly this shape, so a sidecar can forward buckets rather than
// pre-computed percentiles. That distinction is the whole point -- percentiles
// cannot be combined after the fact, but the counts they came from can.
type Histogram map[float64]int64

// Merge adds another histogram's counts into this one.
//
// The source is only read, never retained, so a caller reusing its buffer for
// the next interval cannot corrupt what has already been merged.
func (h Histogram) Merge(other Histogram) {
	for rt, n := range other {
		h[rt] += n
	}
}

// Count is the number of samples recorded.
func (h Histogram) Count() int64 {
	var total int64
	for _, n := range h {
		total += n
	}
	return total
}

// Percentile returns the response time at the given percentile, by nearest
// rank: the smallest recorded time at or below which that share of samples
// falls.
//
// This is the only correct way to get a percentile for a sharded run. Averaging
// the shards' own percentiles under-reports the result -- a tail that one shard
// saw is diluted by the shards that did not -- and it under-reports in the
// direction that makes a target look healthier than it was.
//
// Percentiles outside 0..100 clamp rather than fail: they arrive from
// configuration and reports, and refusing them mid-run would lose the results.
func (h Histogram) Percentile(p float64) float64 {
	total := h.Count()
	if total == 0 {
		return 0
	}
	p = math.Min(math.Max(p, 0), 100)

	buckets := make([]float64, 0, len(h))
	for rt := range h {
		buckets = append(buckets, rt)
	}
	sort.Float64s(buckets)

	rank := int64(math.Ceil(p / 100 * float64(total)))
	if rank < 1 {
		rank = 1
	}

	var seen int64
	for _, rt := range buckets {
		seen += h[rt]
		if seen >= rank {
			return rt
		}
	}
	return buckets[len(buckets)-1]
}

// Percentiles returns several percentiles in one pass over the buckets, which
// is what a report needs and avoids re-sorting for each one.
func (h Histogram) Percentiles(ps ...float64) map[float64]float64 {
	out := make(map[float64]float64, len(ps))
	total := h.Count()
	if total == 0 {
		for _, p := range ps {
			out[p] = 0
		}
		return out
	}

	buckets := make([]float64, 0, len(h))
	for rt := range h {
		buckets = append(buckets, rt)
	}
	sort.Float64s(buckets)

	// Walk the percentiles in order alongside a single pass of the buckets.
	sorted := append([]float64(nil), ps...)
	sort.Float64s(sorted)

	var (
		seen int64
		idx  int
	)
	for _, rt := range buckets {
		seen += h[rt]
		for idx < len(sorted) {
			p := math.Min(math.Max(sorted[idx], 0), 100)
			rank := int64(math.Ceil(p / 100 * float64(total)))
			if rank < 1 {
				rank = 1
			}
			if seen < rank {
				break
			}
			out[sorted[idx]] = rt
			idx++
		}
	}
	for ; idx < len(sorted); idx++ {
		out[sorted[idx]] = buckets[len(buckets)-1]
	}
	return out
}

// MarshalJSON writes the buckets as a JSON object keyed by response time.
//
// Go cannot marshal a float-keyed map on its own -- JSON object keys must be
// strings -- and this is a wire contract with the Python reporter in the engine
// pod, which writes exactly this shape. Without it the sidecar could not send
// what it collected.
func (h Histogram) MarshalJSON() ([]byte, error) {
	out := make(map[string]int64, len(h))
	for rt, n := range h {
		out[strconv.FormatFloat(rt, 'g', -1, 64)] = n
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads buckets keyed by response time.
func (h *Histogram) UnmarshalJSON(data []byte) error {
	var raw map[string]int64
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("metrics: histogram: %w", err)
	}
	out := make(Histogram, len(raw))
	for key, n := range raw {
		rt, err := strconv.ParseFloat(key, 64)
		if err != nil {
			return fmt.Errorf("metrics: histogram key %q is not a response time: %w", key, err)
		}
		out[rt] = n
	}
	*h = out
	return nil
}
