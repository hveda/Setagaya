package scenarioapp

// Line-anchored diagnostics for declarative workload validation. A rejected
// fragment must tell the editor WHERE it is wrong, not just THAT it is wrong:
// yaml.v3 already knows the line of every type mismatch, so throwing that
// away and returning one opaque 400 string would force the operator to diff
// their document against the schema by hand.

import (
	"bytes"
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

// uncompiledKeysMessage is G6's wording contract: the key is PERSISTED (the
// fragment is stored byte-for-byte, think-time included) but compileScenario
// (compile.go) builds a fresh taurus.Scenario from modelled fields only, so
// an unmodelled key never reaches the engine. Earlier phrasing claimed the
// opposite ("passes through to the run") -- a test asserts this wording.
const uncompiledKeysMessage = "this key is stored but not compiled and will not affect the run"

// ModelledButUncompiled names taurus.Scenario fields that KnownFields(true)
// cannot catch, because the type models them, and that a fragment's own
// value never reaches the engine anyway: compileScenario (compile.go) reads
// these from ScenarioInput's own fields, never from the fragment.
//
//   - data-sources: DataSources comes from ScenarioInput.DataPaths, resolved
//     from uploaded scenario files -- never from frag.DataSources.
//   - script: Script comes from ScenarioInput.ScriptPath, and only for a
//     native scenario -- a portable fragment's own "script:" key is never
//     read at all.
//
// This set is a fact about compileScenario, not derivable from the struct,
// so it is exported for phase 19b F3's guard test
// (internal/app/lifecycleapp/service_test.go), which pins it against the
// real compile path and fails the moment a listed field starts being
// compiled from the fragment, or a newly-uncompiled field is not added here.
var ModelledButUncompiled = map[string]struct{}{
	"data-sources": {},
	"script":       {},
}

// uncompiledKeyDiagnostics reports every key a stored-but-not-compiled
// fragment carries, as severity info: both keys taurus.Scenario does not
// model at all (think-time, variables, ...), found via a KnownFields(true)
// decode, and keys the type models but compileScenario never reads from the
// fragment (ModelledButUncompiled, above) -- KnownFields cannot see those,
// since the type accepts them. The fragment is NOT invalid either way --
// ValidateRequests adds these to a 200 with valid:true, and SetRequests
// stores the document unchanged.
func uncompiledKeyDiagnostics(raw []byte) []Diagnostic {
	out := unknownFieldDiagnostics(raw)
	out = append(out, modelledButUncompiledDiagnostics(raw)...)
	return out
}

// unknownFieldDiagnostics finds top-level keys taurus.Scenario does not
// model, via a single KnownFields(true) decode: unknown fields come back as
// TypeError entries with their line numbers, which the same anchoring logic
// the error path uses can parse.
func unknownFieldDiagnostics(raw []byte) []Diagnostic {
	var frag taurus.Scenario
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	err := dec.Decode(&frag)
	if err == nil {
		return nil // every key is modelled
	}
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) || !isUnknownFieldErr(typeErr) {
		return nil // a real parse/type error: the error path reports it
	}
	var out []Diagnostic
	for _, msg := range typeErr.Errors {
		if !strings.Contains(msg, "field") {
			continue
		}
		line, rest := 0, msg
		if m := linePrefix.FindStringSubmatchIndex(msg); m != nil {
			line, _ = strconv.Atoi(msg[m[2]:m[3]])
			rest = strings.TrimSpace(msg[m[1]:])
		}
		rest = strings.TrimPrefix(rest, "field ")
		rest = strings.TrimPrefix(rest, "not found in type ")
		out = append(out, Diagnostic{
			Severity: SeverityInfo,
			Message:  rest + ": " + uncompiledKeysMessage,
			Line:     line,
		})
	}
	return out
}

// modelledButUncompiledDiagnostics finds top-level keys in ModelledButUncompiled
// present in the raw document, line-anchored via a lenient decode into a
// generic node (not KnownFields -- these keys ARE known to taurus.Scenario,
// so a strict decode would never flag them).
func modelledButUncompiledDiagnostics(raw []byte) []Diagnostic {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Content) == 0 {
		return nil // malformed or empty: the error path already reports it
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	var out []Diagnostic
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		if _, ok := ModelledButUncompiled[key.Value]; !ok {
			continue
		}
		out = append(out, Diagnostic{
			Severity: SeverityInfo,
			Message:  key.Value + ": " + uncompiledKeysMessage,
			Line:     key.Line,
			Col:      key.Column,
		})
	}
	return out
}

// isUnknownFieldErr reports whether typeErr is solely about unknown fields.
// If the document ALSO has type errors, the strict decode fails for both
// reasons and we let the ordinary error path handle the report; unknown-key
// info must never mask, or be masked by, a real rejection.
func isUnknownFieldErr(typeErr *yaml.TypeError) bool {
	for _, msg := range typeErr.Errors {
		if !strings.Contains(msg, "field") {
			return false
		}
	}
	return len(typeErr.Errors) > 0
}
