package metrics_test

import (
	"encoding/json"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// Merging is how several pods' reports for the same second become the run's
// measurement for that second.
func TestInterval_Merge(t *testing.T) {
	t.Parallel()

	a := metrics.Interval{
		Timestamp: 100, Label: "checkout-cart",
		Concurrency: 5, Samples: 100, Succeeded: 90, Failed: 10, Bytes: 5000,
		Latency:       metrics.Histogram{0.01: 90, 0.5: 10},
		ResponseCodes: map[string]int64{"200": 90, "500": 10},
		Errors:        []metrics.ErrorGroup{{Message: "Internal Server Error", ResponseCode: "500", Count: 10}},
	}
	b := metrics.Interval{
		Timestamp: 100, Label: "checkout-cart",
		Concurrency: 4, Samples: 80, Succeeded: 80, Failed: 0, Bytes: 4000,
		Latency:       metrics.Histogram{0.01: 80},
		ResponseCodes: map[string]int64{"200": 80},
	}

	a.Merge(b)

	if a.Concurrency != 9 || a.Samples != 180 || a.Succeeded != 170 || a.Failed != 10 || a.Bytes != 9000 {
		t.Errorf("merged counters = %+v", a)
	}
	if a.Latency[0.01] != 170 || a.Latency[0.5] != 10 {
		t.Errorf("merged latency = %+v", a.Latency)
	}
	if a.ResponseCodes["200"] != 170 || a.ResponseCodes["500"] != 10 {
		t.Errorf("merged response codes = %+v", a.ResponseCodes)
	}
	// 10 of 180 merged samples are slow -- 5.6%, which is more than the 5% tail
	// p95 excludes -- so p95 lands in the slow bucket. Neither pod could have
	// reported that alone: one saw 10% slow, the other none.
	if got := a.Latency.Percentile(95); got != 0.5 {
		t.Errorf("merged p95 = %v, want 0.5", got)
	}
	if got := a.Latency.Percentile(94); got != 0.01 {
		t.Errorf("merged p94 = %v, want 0.01 (just inside the fast bucket)", got)
	}
}

// The same failure seen by several pods is one failure with the total count,
// not several entries a reader has to add up.
func TestInterval_MergeCombinesMatchingErrors(t *testing.T) {
	t.Parallel()

	a := metrics.Interval{
		Errors: []metrics.ErrorGroup{{Message: "Internal Server Error", ResponseCode: "500", Count: 3}},
	}
	a.Merge(metrics.Interval{
		Errors: []metrics.ErrorGroup{
			{Message: "Internal Server Error", ResponseCode: "500", Count: 4},
			{Message: "Not Found", ResponseCode: "404", Count: 2},
		},
	})

	if len(a.Errors) != 2 {
		t.Fatalf("errors = %+v, want the 500 combined and the 404 added", a.Errors)
	}
	for _, e := range a.Errors {
		switch e.ResponseCode {
		case "500":
			if e.Count != 7 {
				t.Errorf("500 count = %d, want 7", e.Count)
			}
		case "404":
			if e.Count != 2 {
				t.Errorf("404 count = %d, want 2", e.Count)
			}
		}
	}
}

// A pod that reported nothing for a second must not disturb the aggregate, and
// merging into an empty interval must not panic on nil maps.
func TestInterval_MergeHandlesEmpties(t *testing.T) {
	t.Parallel()

	var zero metrics.Interval
	zero.Merge(metrics.Interval{
		Samples: 5, Latency: metrics.Histogram{0.02: 5},
		ResponseCodes: map[string]int64{"200": 5},
		Errors:        []metrics.ErrorGroup{{Message: "x", Count: 1}},
	})
	if zero.Samples != 5 || zero.Latency[0.02] != 5 || zero.ResponseCodes["200"] != 5 || len(zero.Errors) != 1 {
		t.Errorf("merge into a zero interval = %+v", zero)
	}

	before := zero
	zero.Merge(metrics.Interval{})
	if zero.Samples != before.Samples || len(zero.Errors) != len(before.Errors) {
		t.Errorf("merging an empty interval changed the aggregate: %+v", zero)
	}
}

// The wire format is a contract with the Python reporter in the engine pod:
// buckets arrive as a JSON object keyed by response time, which Go cannot
// marshal from a float-keyed map without help.
func TestInterval_JSONRoundTripsBuckets(t *testing.T) {
	t.Parallel()

	// Exactly the shape the reporter writes.
	raw := `{"ts":1785456662,"label":"probe-ok","concurrency":2,"samples":173,` +
		`"succeeded":148,"failed":25,"bytes":23391,` +
		`"latency":{"0.0":128,"0.001":42,"0.002":3},` +
		`"response_codes":{"200":148,"404":13,"500":12},` +
		`"errors":[{"message":"didn't succeed (404)","response_code":"404","count":13}]}`

	var in metrics.Interval
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatalf("unmarshal reporter output: %v", err)
	}
	if in.Label != "probe-ok" || in.Samples != 173 || in.Bytes != 23391 {
		t.Errorf("decoded = %+v", in)
	}
	if in.Latency[0.001] != 42 || in.Latency.Count() != 173 {
		t.Errorf("latency buckets = %+v (count %d)", in.Latency, in.Latency.Count())
	}
	if in.ResponseCodes["404"] != 13 || len(in.Errors) != 1 {
		t.Errorf("errors/codes = %+v / %+v", in.ResponseCodes, in.Errors)
	}

	out, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var again metrics.Interval
	if err := json.Unmarshal(out, &again); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if again.Latency.Count() != in.Latency.Count() || again.Latency[0.002] != 3 {
		t.Errorf("buckets did not survive a round trip: %+v", again.Latency)
	}
}

func TestHistogram_UnmarshalRejectsNonNumericKeys(t *testing.T) {
	t.Parallel()

	var h metrics.Histogram
	if err := json.Unmarshal([]byte(`{"fast":1}`), &h); err == nil {
		t.Error("a non-numeric bucket key was accepted")
	}
	if err := json.Unmarshal([]byte(`["not","an","object"]`), &h); err == nil {
		t.Error("a non-object histogram was accepted")
	}
}
