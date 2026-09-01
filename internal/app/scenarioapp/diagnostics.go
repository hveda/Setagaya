package scenarioapp

// Line-anchored diagnostics for declarative workload validation. A rejected
// fragment must tell the editor WHERE it is wrong, not just THAT it is wrong:
// yaml.v3 already knows the line of every type mismatch, so throwing that
// away and returning one opaque 400 string would force the operator to diff
// their document against the schema by hand.

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// Severity grades a diagnostic. The editor renders error/warning/info
// distinctly; info findings (uncompiled Taurus keys, phase 19 G6) never
// block storing a fragment.
type Severity string

const (
	// SeverityError marks a document the validation rejects outright.
	SeverityError Severity = "error"
	// SeverityWarning marks something suspicious that does not block
	// storing the fragment.
	SeverityWarning Severity = "warning"
	// SeverityInfo marks a note that never blocks storing: phase 19 G6
	// (uncompiled Taurus keys) uses this to say a key is stored but not
	// compiled and will not affect the run.
	SeverityInfo Severity = "info"
)

// Diagnostic is one finding about a YAML document, anchored to the line it
// came from where yaml.v3 could tell us one. Line and Col are 1-based; 0
// means the finding could not be anchored to a position (e.g. "the fragment
// has no requests at all" has no line to point at), and the caller renders
// it as a document-level error.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Line     int      `json:"line"`
	Col      int      `json:"col"`
	Path     string   `json:"path,omitempty"`
}

// InvalidRequestsError reports a fragment rejected by validation, carrying
// the reasons as line-anchored diagnostics. It wraps ErrRequestsInvalid, so
// every existing errors.Is check (and the HTTP 400 mapping) keeps working;
// the diagnostics ride along for callers that want positions.
type InvalidRequestsError struct {
	Diagnostics []Diagnostic
	Err         error
}

func (e *InvalidRequestsError) Error() string {
	if len(e.Diagnostics) == 0 {
		return e.Err.Error()
	}
	parts := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		if d.Line > 0 {
			parts = append(parts, fmt.Sprintf("line %d: %s", d.Line, d.Message))
			continue
		}
		parts = append(parts, d.Message)
	}
	return e.Err.Error() + ": " + strings.Join(parts, "; ")
}

func (e *InvalidRequestsError) Unwrap() error { return e.Err }

// linePrefix matches yaml.v3's per-error position prefix. TypeError entries
// all begin with "line N: "; parse failures embed the same shape inside the
// message ("yaml: line N: did not find expected ...").
var linePrefix = regexp.MustCompile(`line (\d+): `)

// diagnosticsFromYAMLError converts a yaml.Unmarshal error into
// line-anchored diagnostics. A *yaml.TypeError already carries one entry per
// bad value with its line; anything else (a document that is not YAML at
// all) becomes a single diagnostic from whatever position yaml.v3 put in the
// message -- 0 when there is none.
func diagnosticsFromYAMLError(err error) []Diagnostic {
	if err == nil {
		return nil
	}
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		out := make([]Diagnostic, 0, len(typeErr.Errors))
		for _, msg := range typeErr.Errors {
			line, rest := 0, msg
			if m := linePrefix.FindStringSubmatchIndex(msg); m != nil {
				line, _ = strconv.Atoi(msg[m[2]:m[3]])
				rest = strings.TrimSpace(msg[m[1]:])
			}
			out = append(out, Diagnostic{Severity: SeverityError, Message: rest, Line: line})
		}
		if len(out) > 0 {
			return out
		}
	}
	msg := err.Error()
	line, rest := 0, msg
	if m := linePrefix.FindStringSubmatchIndex(msg); m != nil {
		line, _ = strconv.Atoi(msg[m[2]:m[3]])
		rest = strings.TrimSpace(msg[m[1]:])
	} else {
		rest = strings.TrimPrefix(rest, "yaml: ")
	}
	return []Diagnostic{{Severity: SeverityError, Message: rest, Line: line}}
}

// requestDiagnostics validates a raw fragment exactly the way SetRequests
// always has -- decode into taurus.Scenario, require at least one request,
// require a url on each -- and returns the reasons as diagnostics instead of
// a wrapped yaml error. Empty return means the fragment is acceptable.
//
// This is the single validation path: SetRequests and the phase 19 G5
// validate endpoint both call it, so the two can never disagree about what
// is accepted.
func requestDiagnostics(raw []byte) []Diagnostic {
	var frag taurus.Scenario
	if err := yaml.Unmarshal(raw, &frag); err != nil {
		return diagnosticsFromYAMLError(err)
	}
	if len(frag.Requests) == 0 {
		return []Diagnostic{{
			Severity: SeverityError,
			Message:  "at least one request is required",
			Path:     "requests",
		}}
	}
	var out []Diagnostic
	for i, req := range frag.Requests {
		if req.URL == "" {
			out = append(out, Diagnostic{
				Severity: SeverityError,
				Message:  fmt.Sprintf("request %d has no url", i),
				Path:     fmt.Sprintf("requests[%d].url", i),
			})
		}
	}
	return out
}
