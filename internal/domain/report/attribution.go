package report

import (
	"sort"
	"strconv"
	"strings"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// Side is who a failure belongs to.
//
// This is the most consequential distinction in a load test. An error spike is
// routinely the generator exhausting its own file descriptors, sockets, or heap,
// and reading that as the target failing means declaring a service unfit on the
// strength of a limitation in the thing measuring it.
type Side string

const (
	// SideTarget means the service under test failed, or stopped answering.
	SideTarget Side = "target"
	// SideEngine means the load generator failed, which says nothing about the
	// target.
	SideEngine Side = "engine"
	// SideUnknown means the failure is not attributable. It stays visible rather
	// than being folded into either side.
	SideUnknown Side = "unknown"
)

// engineFaults are phrases that only a generator produces. They are matched
// conservatively: a wrong attribution is worse than an admitted gap, because it
// moves blame between a customer's service and Honryu itself.
var engineFaults = []string{
	"too many open files",
	"cannot assign requested address", // ephemeral ports exhausted
	"outofmemoryerror",
	"out of memory",
	"cannot allocate memory",
	"taurus internal exception",
	"tool error",
	"no space left on device",
}

// targetFaults are ways a target stops answering. They are the target's
// behaviour even though no response arrived -- often exactly what a saturation
// test is looking for.
var targetFaults = []string{
	"connection refused",
	"connection reset",
	"connection timed out",
	"read timed out",
	"i/o timeout",
	"timeout awaiting",
	"no route to host",
	"eof",
}

// AttributeError decides who a failure belongs to.
//
// A response code settles it: the target answered, badly, which is a statement
// about the target. Without one the message is consulted, and anything
// unrecognised stays unknown rather than being guessed.
func AttributeError(e metrics.ErrorGroup) Side {
	if isResponseCode(e.ResponseCode) {
		return SideTarget
	}
	msg := strings.ToLower(e.Message)
	if msg == "" {
		return SideUnknown
	}
	// Engine faults are checked first: a generator that has run out of sockets
	// also reports connection failures, and the resource exhaustion is the
	// truer cause.
	for _, phrase := range engineFaults {
		if strings.Contains(msg, phrase) {
			return SideEngine
		}
	}
	for _, phrase := range targetFaults {
		if strings.Contains(msg, phrase) {
			return SideTarget
		}
	}
	return SideUnknown
}

// isResponseCode reports whether a code is an actual HTTP status rather than an
// engine's placeholder for "no response".
func isResponseCode(code string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(code))
	return err == nil && n >= 100 && n <= 599
}

// Attribution counts a run's failures by side.
//
// All three are reported. Unknown is never folded into target or engine: a
// report that hid it would be claiming a certainty it does not have.
type Attribution struct {
	Target  int64 `json:"target"`
	Engine  int64 `json:"engine"`
	Unknown int64 `json:"unknown"`
}

// Total is every attributed failure.
func (a Attribution) Total() int64 { return a.Target + a.Engine + a.Unknown }

// AttributedError is one failure mode with its origin stated.
type AttributedError struct {
	Side Side `json:"side"`
	// Message is the engine's own wording, kept as an exemplar for a human.
	// Engines word the same failure differently, so it is never a grouping key.
	Message      string `json:"message"`
	ResponseCode string `json:"response_code,omitempty"`
	Count        int64  `json:"count"`
}

// TargetErrorRate is the share of requests the target itself failed.
//
// This is the rate a verdict rests on. The overall rate includes failures the
// generator caused, and failing a sale on those would be failing it for
// Honryu's own limitations.
func (r Report) TargetErrorRate() float64 {
	if r.Achieved.Samples <= 0 {
		return 0
	}
	return float64(r.Attribution.Target) / float64(r.Achieved.Samples)
}

// EngineImpaired reports whether the generator's own failures dominated the run.
//
// Such a run is not evidence about the target, and a reader who took it as such
// would draw the most dangerous conclusion available: that a service failed,
// when the load rig did.
func (r Report) EngineImpaired() bool {
	if r.Attribution.Engine == 0 {
		return false
	}
	return r.Attribution.Engine > r.Attribution.Target
}

// collectErrors combines every pod's failures into one attributed list, ordered
// so the dominant failure is read first.
func collectErrors(intervals []metrics.Interval) ([]AttributedError, Attribution) {
	type key struct{ code, message string }
	merged := map[key]*AttributedError{}

	for _, iv := range intervals {
		if iv.Label == TotalLabel {
			continue // the engine's own aggregate; already counted per label
		}
		for _, e := range iv.Errors {
			k := key{e.ResponseCode, e.Message}
			existing, ok := merged[k]
			if !ok {
				existing = &AttributedError{
					Side:         AttributeError(e),
					Message:      e.Message,
					ResponseCode: e.ResponseCode,
				}
				merged[k] = existing
			}
			existing.Count += e.Count
		}
	}

	out := make([]AttributedError, 0, len(merged))
	var attr Attribution
	for _, e := range merged {
		out = append(out, *e)
		switch e.Side {
		case SideTarget:
			attr.Target += e.Count
		case SideEngine:
			attr.Engine += e.Count
		default:
			attr.Unknown += e.Count
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].ResponseCode != out[j].ResponseCode {
			return out[i].ResponseCode < out[j].ResponseCode
		}
		return out[i].Message < out[j].Message
	})
	return out, attr
}
