package report

import (
	"regexp"
	"strconv"
	"strings"
)

// FailedCriterion is one configured pass/fail criterion that triggered (a
// failure, matching bzt's own passfail semantics: "failures>10%" means the
// run fails when failures actually exceed 10%), or whose subject/syntax
// fell outside the practical grammar EvaluateCriteria understands.
type FailedCriterion struct {
	// Criterion is the original configured expression, unmodified.
	Criterion string
	// Unparsed is true when Criterion could not be evaluated at all --
	// its outcome is unknown, not confirmed triggered. Never silently
	// dropped, and never reported as if it had failed when it merely
	// couldn't be read.
	Unparsed bool
}

// criterionPattern matches the practical subset of bzt's own criteria
// grammar EvaluateCriteria understands: a bare "subject operator threshold"
// comparison, with an optional %, ms, or s unit suffix on the threshold.
// bzt's fuller grammar (a time window via "for", a label filter, a
// "stop as" clause, multiple ANDed conditions) is out of scope -- reported
// as unparsed rather than misread.
var criterionPattern = regexp.MustCompile(`^\s*([a-zA-Z0-9_-]+)\s*(>=|<=|==|>|<|=)\s*([0-9]+(?:\.[0-9]+)?)\s*(%|ms|s)?\s*$`)

// EvaluateCriteria checks each of criteria (Taurus pass/fail expressions,
// e.g. "failures>10%", "p95>500ms") against r's own measured fields, and
// returns every one that triggered, or whose subject/syntax fell outside
// the practical subset understood. A criterion that did not trigger is
// omitted entirely -- the result names only what actually failed (or
// couldn't be read), never the full configured list.
//
// Supported subjects, the only ones r has data for: "failures"/"fail"
// (against ErrorRate, percent threshold required -- a bare number would be
// ambiguous between a fraction and a percentage), and "pNN" (e.g. "p95",
// against whichever percentile keys Latency actually carries; a threshold
// with no unit suffix defaults to milliseconds, bzt's own usual convention
// for response-time criteria).
func (r Report) EvaluateCriteria(criteria []string) []FailedCriterion {
	var out []FailedCriterion
	for _, c := range criteria {
		if fc := r.evaluateOne(c); fc != nil {
			out = append(out, *fc)
		}
	}
	return out
}

func (r Report) evaluateOne(c string) *FailedCriterion {
	m := criterionPattern.FindStringSubmatch(c)
	if m == nil {
		return &FailedCriterion{Criterion: c, Unparsed: true}
	}
	subject, op, unit := strings.ToLower(m[1]), m[2], m[4]
	rawThreshold, err := strconv.ParseFloat(m[3], 64)
	if err != nil {
		return &FailedCriterion{Criterion: c, Unparsed: true}
	}

	var measured, threshold float64
	switch {
	case subject == "failures" || subject == "fail":
		if unit != "%" {
			return &FailedCriterion{Criterion: c, Unparsed: true}
		}
		measured = r.ErrorRate * 100
		threshold = rawThreshold
	case strings.HasPrefix(subject, "p"):
		pct, perr := strconv.ParseFloat(subject[1:], 64)
		if perr != nil {
			return &FailedCriterion{Criterion: c, Unparsed: true}
		}
		latency, ok := r.Latency[pct]
		if !ok {
			return &FailedCriterion{Criterion: c, Unparsed: true}
		}
		measured = latency
		switch unit {
		case "ms", "":
			threshold = rawThreshold / 1000
		case "s":
			threshold = rawThreshold
		default:
			return &FailedCriterion{Criterion: c, Unparsed: true}
		}
	default:
		return &FailedCriterion{Criterion: c, Unparsed: true}
	}

	if criterionTriggered(measured, op, threshold) {
		return &FailedCriterion{Criterion: c}
	}
	return nil
}

// criterionTriggered reports whether the criterion's own comparison holds.
func criterionTriggered(measured float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return measured > threshold
	case ">=":
		return measured >= threshold
	case "<":
		return measured < threshold
	case "<=":
		return measured <= threshold
	case "=", "==":
		return measured == threshold
	default:
		return false
	}
}
