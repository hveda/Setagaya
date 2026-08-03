package report_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
)

// The question every load test has to answer before its results mean anything:
// did the generator break, or did the target?
func TestAttributeError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  metrics.ErrorGroup
		want report.Side
	}{
		// A response code means the target answered. It answered badly, but it
		// answered -- that is a statement about the target.
		{
			"http 500 from JMeter",
			metrics.ErrorGroup{Message: "Internal Server Error", ResponseCode: "500"},
			report.SideTarget,
		},
		{
			"http 404 from apiritif",
			metrics.ErrorGroup{Message: "Request to http://svc/cart didn't succeed (404)", ResponseCode: "404"},
			report.SideTarget,
		},
		{
			"http 503 from k6",
			metrics.ErrorGroup{Message: "Response code: 503", ResponseCode: "503"},
			report.SideTarget,
		},
		// The target refused or stopped answering: still the target's behaviour,
		// and often exactly what a saturation test is looking for.
		{
			"connection refused",
			metrics.ErrorGroup{Message: "dial tcp 10.0.0.1:80: connect: connection refused"},
			report.SideTarget,
		},
		{
			"read timeout",
			metrics.ErrorGroup{Message: "java.net.SocketTimeoutException: Read timed out"},
			report.SideTarget,
		},
		{
			"connection reset",
			metrics.ErrorGroup{Message: "Connection reset by peer"},
			report.SideTarget,
		},
		// The generator ran out of something. Nothing here says anything about
		// the target, and counting it as a target failure would fail a service
		// that was never actually stressed.
		{
			"file descriptors",
			metrics.ErrorGroup{Message: "socket: too many open files"},
			report.SideEngine,
		},
		{
			"ephemeral ports",
			metrics.ErrorGroup{Message: "connect: cannot assign requested address"},
			report.SideEngine,
		},
		{
			"engine heap",
			metrics.ErrorGroup{Message: "java.lang.OutOfMemoryError: Java heap space"},
			report.SideEngine,
		},
		{
			"engine crashed",
			metrics.ErrorGroup{Message: "Taurus internal exception: ToolError"},
			report.SideEngine,
		},
		// Anything unrecognised stays unknown rather than being guessed into a
		// side. A wrong attribution is worse than an admitted gap: it is the
		// difference between blaming a service and blaming your own load rig.
		{
			"unrecognised",
			metrics.ErrorGroup{Message: "something went sideways"},
			report.SideUnknown,
		},
		{"empty", metrics.ErrorGroup{}, report.SideUnknown},
		// A non-numeric code is not a response.
		{
			"non-numeric code",
			metrics.ErrorGroup{Message: "boom", ResponseCode: "N/A"},
			report.SideUnknown,
		},
		// apiritif's messages carry the request URL, and a bare "eof" substring
		// match would fire inside an ordinary word a URL can contain. An
		// unrecognised failure against such a URL must stay unknown, not be
		// guessed into target just because of what the path happens to spell.
		{
			"unrecognised failure against a url containing eof as a substring",
			metrics.ErrorGroup{Message: "Request to http://svc/geofence/v1 didn't succeed: something went sideways"},
			report.SideUnknown,
		},
		{
			"unrecognised failure against a url spelling eof another way",
			metrics.ErrorGroup{Message: "Request to http://svc/videofeed didn't succeed: something went sideways"},
			report.SideUnknown,
		},
		// The real wordings a dropped connection produces, specific enough that
		// a URL cannot spell them by accident.
		{
			"jmeter EOFException",
			metrics.ErrorGroup{Message: "java.io.EOFException: null"},
			report.SideTarget,
		},
		{
			"unexpected eof reading body",
			metrics.ErrorGroup{Message: "unexpected EOF while reading response"},
			report.SideTarget,
		},
		{
			"remote end closed the connection",
			metrics.ErrorGroup{Message: "('Connection aborted.', RemoteDisconnected('Remote end closed connection without response'))"},
			report.SideTarget,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := report.AttributeError(tc.err); got != tc.want {
				t.Errorf("AttributeError(%q, rc=%q) = %q, want %q",
					tc.err.Message, tc.err.ResponseCode, got, tc.want)
			}
		})
	}
}

// Attribution is case-insensitive: engines phrase the same failure differently
// and none of them agree on capitalisation.
func TestAttributeError_IgnoresCase(t *testing.T) {
	t.Parallel()

	for _, msg := range []string{
		"Too Many Open Files", "TOO MANY OPEN FILES", "too many open files",
	} {
		if got := report.AttributeError(metrics.ErrorGroup{Message: msg}); got != report.SideEngine {
			t.Errorf("AttributeError(%q) = %q, want engine", msg, got)
		}
	}
}

func TestBuild_AttributesErrors(t *testing.T) {
	t.Parallel()

	in := report.Input{
		ExecutionID: 1, RunID: 1,
		Requested: report.Load{DurationSeconds: 10},
		Intervals: []metrics.Interval{
			{
				Timestamp: 1, Label: "probe", Samples: 100, Succeeded: 70, Failed: 30,
				Latency: metrics.Histogram{0.01: 100},
				Errors: []metrics.ErrorGroup{
					{Message: "Internal Server Error", ResponseCode: "500", Count: 20},
					{Message: "socket: too many open files", Count: 8},
					{Message: "something went sideways", Count: 2},
				},
			},
		},
	}
	rep := report.Build(in)

	if rep.Attribution.Target != 20 {
		t.Errorf("target-side errors = %d, want 20", rep.Attribution.Target)
	}
	if rep.Attribution.Engine != 8 {
		t.Errorf("engine-side errors = %d, want 8", rep.Attribution.Engine)
	}
	if rep.Attribution.Unknown != 2 {
		t.Errorf("unattributed errors = %d, want 2", rep.Attribution.Unknown)
	}

	// Every error group carries its side, so a reader never sees a count whose
	// origin is unstated.
	if len(rep.Errors) != 3 {
		t.Fatalf("error groups = %d, want 3", len(rep.Errors))
	}
	for _, e := range rep.Errors {
		if e.Side == "" {
			t.Errorf("error %q has no side", e.Exemplars)
		}
	}
}

// The rate a verdict rests on counts only what the target did. A run whose
// generator ran out of sockets says nothing about the service, and failing a
// sale on that would be failing it for Honryu's own limitations.
func TestReport_TargetErrorRateExcludesEngineFaults(t *testing.T) {
	t.Parallel()

	in := report.Input{
		ExecutionID: 1, RunID: 1,
		Requested: report.Load{DurationSeconds: 10},
		Intervals: []metrics.Interval{
			{
				Timestamp: 1, Label: "probe", Samples: 100, Succeeded: 50, Failed: 50,
				Latency: metrics.Histogram{0.01: 100},
				Errors: []metrics.ErrorGroup{
					{Message: "socket: too many open files", Count: 45},
					{Message: "Internal Server Error", ResponseCode: "500", Count: 5},
				},
			},
		},
	}
	rep := report.Build(in)

	// Half the requests failed, but only a twentieth were the target's doing.
	if rep.ErrorRate < 0.49 || rep.ErrorRate > 0.51 {
		t.Errorf("overall error rate = %v, want 0.5", rep.ErrorRate)
	}
	if got := rep.TargetErrorRate(); got < 0.049 || got > 0.051 {
		t.Errorf("target error rate = %v, want 0.05", got)
	}
	if rep.TargetErrorRate() >= rep.ErrorRate {
		t.Error("the target error rate is not lower than the overall one")
	}
}

// A run whose failures were mostly the generator's own is not evidence about the
// target, and a report that let someone read it as such would be the most
// dangerous kind of wrong.
func TestReport_EngineImpaired(t *testing.T) {
	t.Parallel()

	build := func(engineErrs, targetErrs int64) report.Report {
		return report.Build(report.Input{
			ExecutionID: 1, RunID: 1,
			Requested: report.Load{DurationSeconds: 10},
			Intervals: []metrics.Interval{{
				Timestamp: 1, Label: "probe", Samples: 100,
				Failed:  engineErrs + targetErrs,
				Latency: metrics.Histogram{0.01: 100},
				Errors: []metrics.ErrorGroup{
					{Message: "socket: too many open files", Count: engineErrs},
					{Message: "Internal Server Error", ResponseCode: "500", Count: targetErrs},
				},
			}},
		})
	}

	if !build(30, 5).EngineImpaired() {
		t.Error("a run where the generator produced most failures is not flagged")
	}
	if build(1, 40).EngineImpaired() {
		t.Error("a run whose failures were the target's is flagged as engine-impaired")
	}
	// A clean run is not impaired.
	if build(0, 0).EngineImpaired() {
		t.Error("a run with no errors is flagged as engine-impaired")
	}
}

// The same failure seen by several pods is one group with the total, not one
// entry per pod that a reader has to add up.
func TestBuild_CombinesMatchingErrorsAcrossPods(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1,
		Requested: report.Load{DurationSeconds: 10},
		Intervals: []metrics.Interval{
			{
				Timestamp: 1, Label: "probe", Samples: 50, Failed: 10,
				Errors: []metrics.ErrorGroup{{Message: "Internal Server Error", ResponseCode: "500", Count: 10}},
			},
			{
				Timestamp: 1, Label: "probe", Samples: 50, Failed: 15,
				Errors: []metrics.ErrorGroup{
					{Message: "Internal Server Error", ResponseCode: "500", Count: 12},
					{Message: "Not Found", ResponseCode: "404", Count: 3},
				},
			},
		},
	})

	if len(rep.Errors) != 2 {
		t.Fatalf("error groups = %d, want 2: %+v", len(rep.Errors), rep.Errors)
	}
	byCode := map[string]int64{}
	for _, e := range rep.Errors {
		byCode[e.ResponseCode] = e.Count
	}
	if byCode["500"] != 22 {
		t.Errorf("500 count = %d, want 22 combined across pods", byCode["500"])
	}
	if byCode["404"] != 3 {
		t.Errorf("404 count = %d, want 3", byCode["404"])
	}
}

// Errors are ordered by how much they happened, so the dominant failure is the
// first thing read rather than something to hunt for.
func TestBuild_OrdersErrorsByCount(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1,
		Requested: report.Load{DurationSeconds: 10},
		Intervals: []metrics.Interval{{
			Timestamp: 1, Label: "probe", Samples: 100, Failed: 60,
			Errors: []metrics.ErrorGroup{
				{Message: "Not Found", ResponseCode: "404", Count: 5},
				{Message: "Internal Server Error", ResponseCode: "500", Count: 50},
				{Message: "Bad Gateway", ResponseCode: "502", Count: 5},
			},
		}},
	})

	if rep.Errors[0].Count != 50 {
		t.Errorf("first error has count %d, want the dominant 50", rep.Errors[0].Count)
	}
	// Ties break by code so two reports of the same run agree.
	if rep.Errors[1].ResponseCode != "404" || rep.Errors[2].ResponseCode != "502" {
		t.Errorf("tied errors are not in a stable order: %+v", rep.Errors[1:])
	}
}

// The wordings the three engines actually produced in the Phase 0 spike, run
// through the classifier. They disagree on phrasing for the same failure, which
// is exactly why the response code decides and the message does not.
func TestAttributeError_RealEngineWordings(t *testing.T) {
	t.Parallel()

	// Captured from bzt 1.16.51 driving the same stub target.
	real := []metrics.ErrorGroup{
		{Message: "Request to http://127.0.0.1:8080/ok didn't succeed (404)", ResponseCode: "404"}, // apiritif
		{Message: "Request to http://127.0.0.1:8080/ok didn't succeed (500)", ResponseCode: "500"}, // apiritif
		{Message: "Not Found", ResponseCode: "404"},                                                // JMeter
		{Message: "Internal Server Error", ResponseCode: "500"},                                    // JMeter
		{Message: "Response code: 404", ResponseCode: "404"},                                       // k6
		{Message: "Response code: 500", ResponseCode: "500"},                                       // k6
		{Message: "Unsupported method ('POST')", ResponseCode: "501"},                              // stub, via JMeter
	}
	for _, e := range real {
		if got := report.AttributeError(e); got != report.SideTarget {
			t.Errorf("AttributeError(%q, rc=%q) = %q, want target -- the target answered",
				e.Message, e.ResponseCode, got)
		}
	}
}

func TestAttribution_Total(t *testing.T) {
	t.Parallel()

	a := report.Attribution{Target: 5, Engine: 3, Unknown: 2}
	if a.Total() != 10 {
		t.Errorf("Total() = %d, want 10", a.Total())
	}
	if (report.Attribution{}).Total() != 0 {
		t.Error("an empty attribution does not total zero")
	}
}

// A run that measured nothing has no rate to report, and dividing by its zero
// samples would put a NaN in front of someone making a go/no-go call.
func TestReport_TargetErrorRateOnAnEmptyRun(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{ExecutionID: 1, RunID: 1})
	if got := rep.TargetErrorRate(); got != 0 {
		t.Errorf("TargetErrorRate() = %v on a run with no samples, want 0", got)
	}
}

// bzt's aggregate label carries the same errors as the requests it summarises,
// so counting its errors too would double every attribution.
func TestBuild_IgnoresErrorsOnTheAggregateLabel(t *testing.T) {
	t.Parallel()

	rep := report.Build(report.Input{
		ExecutionID: 1, RunID: 1,
		Requested: report.Load{DurationSeconds: 10},
		Intervals: []metrics.Interval{
			{
				Timestamp: 1, Label: "probe", Samples: 100, Failed: 10,
				Errors: []metrics.ErrorGroup{{Message: "Internal Server Error", ResponseCode: "500", Count: 10}},
			},
			{
				Timestamp: 1, Label: report.TotalLabel, Samples: 100, Failed: 10,
				Errors: []metrics.ErrorGroup{{Message: "Internal Server Error", ResponseCode: "500", Count: 10}},
			},
		},
	})

	if rep.Attribution.Target != 10 {
		t.Errorf("target errors = %d, want 10; the aggregate label was counted too",
			rep.Attribution.Target)
	}
}
