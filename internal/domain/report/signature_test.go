package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
)

// The finding this whole file exists for: Phase 0 ran the same 404 through three
// engines and got three sentences back. If wording were the grouping key, one
// failure would appear as three, and swapping engines would silently rewrite a
// service's error history.
func TestSignatureCollapsesEngineWordingForTheSame404(t *testing.T) {
	t.Parallel()

	variants := []string{
		"Request to http://svc/cart didn't succeed (404)", // apiritif
		"Not Found",          // JMeter
		"Response code: 404", // k6
	}

	var got []report.Signature
	for _, msg := range variants {
		got = append(got, report.NewSignature("checkout-cart", metrics.ErrorGroup{
			Message:      msg,
			ResponseCode: "404",
		}))
	}

	for _, sig := range got[1:] {
		if sig != got[0] {
			t.Fatalf("engine wording changed the signature: %v != %v", sig, got[0])
		}
	}
	if got[0].Label != "checkout-cart" || got[0].ResponseCode != "404" {
		t.Errorf("signature lost its key: %+v", got[0])
	}
	if got[0].Side != report.SideTarget {
		t.Errorf("side = %q, want %q", got[0].Side, report.SideTarget)
	}
}

// The same collapse, seen through a whole run: three pods on three engines
// reporting the same failure must produce one line in the report, not three.
func TestBuildCollapsesEngineWordingIntoOneSignature(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1,
		Outcome: "failed",
		Intervals: []metrics.Interval{
			errInterval("checkout-cart", 10, 2, metrics.ErrorGroup{
				Message: "Request to http://svc/cart didn't succeed (404)", ResponseCode: "404", Count: 2,
			}),
			errInterval("checkout-cart", 10, 3, metrics.ErrorGroup{
				Message: "Not Found", ResponseCode: "404", Count: 3,
			}),
			errInterval("checkout-cart", 10, 5, metrics.ErrorGroup{
				Message: "Response code: 404", ResponseCode: "404", Count: 5,
			}),
		},
	})

	if len(rep.Errors) != 1 {
		t.Fatalf("got %d error signatures, want 1: %+v", len(rep.Errors), rep.Errors)
	}
	if got := rep.Errors[0].Count; got != 10 {
		t.Errorf("count = %d, want 10 -- counts must survive the collapse", got)
	}
	if got := rep.Attribution.Target; got != 10 {
		t.Errorf("target attribution = %d, want 10", got)
	}
	// Every wording is still readable; none of them is the key.
	if len(rep.Errors[0].Exemplars) != 3 {
		t.Errorf("exemplars = %q, want all three wordings kept", rep.Errors[0].Exemplars)
	}
}

// What must NOT collapse. A signature that merged these would hide which request
// broke, which status it broke with, or who caused it.
func TestSignaturesStaySeparate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		label string
		err   metrics.ErrorGroup
	}{
		{"baseline", "checkout-cart", metrics.ErrorGroup{Message: "Not Found", ResponseCode: "404"}},
		{"different label", "checkout-pay", metrics.ErrorGroup{Message: "Not Found", ResponseCode: "404"}},
		{"different code", "checkout-cart", metrics.ErrorGroup{Message: "Not Found", ResponseCode: "500"}},
		// No response code, and the message decides the side. Two failures that
		// belong to opposite sides must never share a signature: merging them
		// would put the generator's own exhaustion into the target's error count.
		{"engine-side, no code", "checkout-cart", metrics.ErrorGroup{Message: "socket: too many open files"}},
		{"target-side, no code", "checkout-cart", metrics.ErrorGroup{Message: "connection refused"}},
	}

	seen := map[report.Signature]string{}
	for _, tc := range cases {
		sig := report.NewSignature(tc.label, tc.err)
		if prev, dup := seen[sig]; dup {
			t.Errorf("%q collided with %q on signature %v", tc.name, prev, sig)
		}
		seen[sig] = tc.name
	}
}

// Errors carry whatever the target echoed back -- a stack trace, an HTML error
// page. Storing them unbounded would let a misbehaving target dictate the size
// of every report of it.
func TestExemplarsAreBounded(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("stack frame ", 500)
	var intervals []metrics.Interval
	for i := 0; i < 20; i++ {
		intervals = append(intervals, errInterval("checkout-cart", 10, 1, metrics.ErrorGroup{
			// Distinct wording each time: the worst case for exemplar growth.
			Message: huge + string(rune('a'+i)), ResponseCode: "500", Count: 1,
		}))
	}

	rep := report.Build(report.Input{ExecutionID: 1, RunID: 1, Outcome: "failed", Intervals: intervals})

	if len(rep.Errors) != 1 {
		t.Fatalf("got %d signatures, want 1", len(rep.Errors))
	}
	got := rep.Errors[0]
	if got.Count != 20 {
		t.Errorf("count = %d, want 20 -- bounding exemplars must not lose counts", got.Count)
	}
	if len(got.Exemplars) > report.MaxExemplars {
		t.Errorf("kept %d exemplars, want at most %d", len(got.Exemplars), report.MaxExemplars)
	}
	for _, ex := range got.Exemplars {
		if len([]rune(ex)) > report.MaxExemplarLen {
			t.Errorf("exemplar is %d runes, want at most %d", len([]rune(ex)), report.MaxExemplarLen)
		}
	}
}

// Two engines wording a failure identically should not fill the exemplar budget
// with the same sentence.
func TestExemplarsAreDeduplicated(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1, Outcome: "failed",
		Intervals: []metrics.Interval{
			errInterval("checkout-cart", 10, 1, metrics.ErrorGroup{Message: "Not Found", ResponseCode: "404", Count: 1}),
			errInterval("checkout-cart", 10, 1, metrics.ErrorGroup{Message: "Not Found", ResponseCode: "404", Count: 1}),
		},
	})

	if len(rep.Errors) != 1 {
		t.Fatalf("got %d signatures, want 1", len(rep.Errors))
	}
	if got := rep.Errors[0].Exemplars; len(got) != 1 {
		t.Errorf("exemplars = %q, want one distinct wording", got)
	}
}

// An engine that reports a failing code with no wording still produces a
// countable failure. The signature carries it; there is simply nothing to quote.
func TestSignatureWithoutAWording(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1, Outcome: "failed",
		Intervals: []metrics.Interval{
			errInterval("checkout-cart", 10, 2, metrics.ErrorGroup{ResponseCode: "503", Count: 2}),
		},
	})

	if len(rep.Errors) != 1 {
		t.Fatalf("got %d signatures, want 1", len(rep.Errors))
	}
	if got := rep.Errors[0]; got.Count != 2 || len(got.Exemplars) != 0 {
		t.Errorf("error = %+v, want count 2 and no exemplars", got)
	}
	if got := rep.Attribution.Target; got != 2 {
		t.Errorf("target attribution = %d, want 2 -- a code is the target answering", got)
	}
}

// A signature is a storage key (task 28) and an analytics key (Phase 9), so it
// has to survive being written down and read back the same.
func TestSignatureStringIsStable(t *testing.T) {
	t.Parallel()

	sig := report.NewSignature("checkout-cart", metrics.ErrorGroup{Message: "Not Found", ResponseCode: "404"})
	if got, want := sig.String(), "target|404|checkout-cart"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	noCode := report.NewSignature("checkout-cart", metrics.ErrorGroup{Message: "connection refused"})
	if got, want := noCode.String(), "target||checkout-cart"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// The signature is embedded in ErrorSignature, so its fields flatten into the
// same JSON object. A report crosses both the API and the database, and a
// signature that did not survive that trip could not be matched across runs.
func TestErrorSignatureJSONRoundTrip(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1, Outcome: "failed",
		Intervals: []metrics.Interval{
			errInterval("checkout-cart", 10, 4, metrics.ErrorGroup{
				Message: "Not Found", ResponseCode: "404", Count: 4,
			}),
		},
	})

	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Flattened, not nested under a "signature" key.
	if strings.Contains(string(data), `"signature"`) {
		t.Errorf("signature is nested rather than flattened: %s", data)
	}

	var got report.Report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %+v, want 1", got.Errors)
	}
	if got.Errors[0].Signature != rep.Errors[0].Signature {
		t.Errorf("signature = %+v, want %+v", got.Errors[0].Signature, rep.Errors[0].Signature)
	}
	if got.Errors[0].Count != 4 || len(got.Errors[0].Exemplars) != 1 {
		t.Errorf("error lost detail: %+v", got.Errors[0])
	}
}

func errInterval(label string, samples, failed int64, errs ...metrics.ErrorGroup) metrics.Interval {
	return metrics.Interval{
		Label:   label,
		Samples: samples,
		Failed:  failed,
		Errors:  errs,
	}
}
