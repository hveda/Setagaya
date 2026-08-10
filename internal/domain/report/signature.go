package report

import (
	"strings"

	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// Exemplar bounds. A target under stress echoes back whatever it likes -- stack
// traces, HTML error pages -- and a failing run produces the most of it. Without
// a bound the size of a report would be dictated by the thing it is reporting
// on, and reports are kept forever.
const (
	// MaxExemplars is how many distinct wordings are kept per signature. Enough
	// to see that engines disagree, few enough to bound the row.
	MaxExemplars = 3
	// MaxExemplarLen is the longest wording kept, in runes.
	MaxExemplarLen = 300
)

// Signature identifies a failure mode independently of how an engine worded it.
//
// Phase 0 put the same 404 through three engines and got three sentences back:
// "Request to ... didn't succeed (404)", "Not Found", "Response code: 404".
// Response codes, by contrast, were identical across all three. So a signature
// keys on what is stable -- the label Honryu assigned when it compiled the
// config, and the code the target returned -- and never on message text. Group
// by wording instead and one failure appears as three, while re-running the same
// scenario on another engine breaks its error history in two.
type Signature struct {
	// Label is the request that failed, as Honryu named it.
	Label string `json:"label"`
	// ResponseCode is the status the target returned, empty where none arrived.
	ResponseCode string `json:"response_code,omitempty"`
	// Side is part of the key, not just a property of it. Where no response code
	// arrived the side is derived from the message, and two failures on opposite
	// sides -- the generator out of sockets, the target refusing connections --
	// would otherwise merge into one signature and put the generator's own
	// exhaustion into the target's error count.
	Side Side `json:"side"`
}

// NewSignature derives the signature of one engine-reported failure.
//
// Only a real HTTP status is kept as the response code; anything else is
// treated as "no response arrived" and dropped to empty. An engine faced with
// a connection failure does not report a status -- it reports a placeholder,
// and JMeter's placeholder is a whole sentence ("Non HTTP response code:
// org.apache.http...") that both overflows the response-code field's bound and
// is not a code at all. The failure is not lost: its side is still attributed
// from the message, and the message is preserved verbatim as an exemplar.
func NewSignature(label string, e metrics.ErrorGroup) Signature {
	code := strings.TrimSpace(e.ResponseCode)
	if !isResponseCode(code) {
		code = ""
	}
	return Signature{
		Label:        label,
		ResponseCode: code,
		Side:         AttributeError(e),
	}
}

// String is the signature's textual form, for logs, messages, and anywhere one
// failure mode has to be named in a single string.
//
// It is not the identity. Signature is comparable and is the map key here and
// three indexed columns in the database, so matching a failure across runs
// compares the fields. Labels come from user-named scenarios and may contain the
// separator, which would make two signatures read alike -- harmless in a log
// line, wrong in a key.
func (s Signature) String() string {
	return string(s.Side) + "|" + s.ResponseCode + "|" + s.Label
}

// ErrorSignature is one failure mode across a whole run: how it is identified,
// how often it happened, and a bounded sample of how engines described it.
type ErrorSignature struct {
	Signature
	Count int64 `json:"count"`
	// Exemplars are engine wordings kept for a human to read. They are evidence,
	// never identity -- see Signature.
	Exemplars []string `json:"exemplars,omitempty"`
}

// MergeExemplars combines two sets of wordings under the same bound, keeping
// them in the order they were first seen.
//
// A signature is accumulated in pieces as a run reports, so the wordings already
// kept have to be merged with newly measured ones -- and the bound has to hold
// across that merge rather than only within one batch. Keeping the rule here
// means a store cannot accidentally widen it.
func MergeExemplars(kept, incoming []string) []string {
	var merged ErrorSignature
	for _, msg := range kept {
		merged.addExemplar(msg)
	}
	for _, msg := range incoming {
		merged.addExemplar(msg)
	}
	return merged.Exemplars
}

// addExemplar keeps a wording if it is new and there is room, truncating it to
// the bound. Counts are unaffected: a dropped exemplar loses a sentence, never a
// failure.
func (e *ErrorSignature) addExemplar(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" || len(e.Exemplars) >= MaxExemplars {
		return
	}
	if runes := []rune(msg); len(runes) > MaxExemplarLen {
		msg = string(runes[:MaxExemplarLen])
	}
	for _, seen := range e.Exemplars {
		if seen == msg {
			return
		}
	}
	e.Exemplars = append(e.Exemplars, msg)
}
