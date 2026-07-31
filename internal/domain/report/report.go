// Package report turns the measurements a run produced into what is said about
// it afterwards.
//
// A report outlives the engines, the metrics backend's retention, and the
// campaign it belonged to, because it is the evidence a readiness judgement was
// made on. Percentiles come from the merged buckets of every pod, since a
// sharded run's latency cannot be recovered from the pods' own percentiles.
//
// Pure domain: no I/O.
package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// TotalLabel is the label bzt uses for its own aggregate across all requests.
// It is excluded from a report: counting it beside the requests it summarises
// would double every total.
const TotalLabel = "__total__"

// Validation errors. Callers compare with errors.Is.
var (
	ErrExecutionRequired = errors.New("report: a valid execution id is required")
	ErrRunRequired       = errors.New("report: a valid run id is required")
	ErrOutcomeUnknown    = errors.New("report: unknown outcome")
)

// Percentiles maps a percentile to the response time at it.
//
// Like a Histogram it needs explicit marshalling: Go cannot marshal a
// float-keyed map, and JSON object keys must be strings. A report crosses both
// the API and the database, so without this it could be built and never served.
type Percentiles map[float64]float64

// MarshalJSON writes percentiles keyed by their number ("95": 0.2).
func (p Percentiles) MarshalJSON() ([]byte, error) {
	out := make(map[string]float64, len(p))
	for pct, v := range p {
		out[strconv.FormatFloat(pct, 'g', -1, 64)] = v
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads percentiles keyed by their number.
func (p *Percentiles) UnmarshalJSON(data []byte) error {
	var raw map[string]float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("report: percentiles: %w", err)
	}
	out := make(Percentiles, len(raw))
	for key, v := range raw {
		pct, err := strconv.ParseFloat(key, 64)
		if err != nil {
			return fmt.Errorf("report: percentile key %q is not a number: %w", key, err)
		}
		out[pct] = v
	}
	*p = out
	return nil
}

// reportedPercentiles are the percentiles every report carries.
var reportedPercentiles = []float64{50, 90, 95, 99}

// Load is a rate of work: what was asked for, or what was produced.
type Load struct {
	// Concurrency is virtual users.
	Concurrency int `json:"concurrency"`
	// Throughput is requests per second. Zero means unlimited when requested,
	// and nothing measured when achieved.
	Throughput float64 `json:"throughput"`
	// DurationSeconds is how long the load was meant to hold.
	DurationSeconds int `json:"duration_seconds,omitempty"`
	// Samples and Failed count requests; only meaningful for achieved load.
	Samples int64 `json:"samples,omitempty"`
	Failed  int64 `json:"failed,omitempty"`
}

// LabelSummary is one request's share of a run. A service owner needs to know
// which request degraded, not only that something did.
type LabelSummary struct {
	Label     string      `json:"label"`
	Samples   int64       `json:"samples"`
	Failed    int64       `json:"failed"`
	ErrorRate float64     `json:"error_rate"`
	Latency   Percentiles `json:"latency"`
}

// Input is everything a report is built from.
type Input struct {
	ExecutionID int64
	ScenarioID  int64
	RunID       int64
	Engine      taurus.Executor
	StartedAt   time.Time
	EndedAt     time.Time
	// Requested is the load the execution asked for.
	Requested Load
	// Outcome is how the run ended.
	Outcome taurus.Outcome
	// Intervals are every second every pod reported.
	Intervals []metrics.Interval
}

// Report is what a run is judged on.
type Report struct {
	ExecutionID int64           `json:"execution_id"`
	ScenarioID  int64           `json:"scenario_id"`
	RunID       int64           `json:"run_id"`
	Engine      taurus.Executor `json:"engine,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	EndedAt     time.Time       `json:"ended_at"`
	Outcome     taurus.Outcome  `json:"outcome"`

	// Requested and Achieved sit side by side deliberately: latency means little
	// without knowing whether the load that produced it was the load intended.
	Requested Load `json:"requested"`
	Achieved  Load `json:"achieved"`

	// ErrorRate counts every failure, whoever caused it. It is not a verdict
	// input on its own -- see TargetErrorRate, which counts only the target's.
	ErrorRate float64     `json:"error_rate"`
	Latency   Percentiles `json:"latency"`

	// Attribution splits the failures by who caused them, and Errors states the
	// side of each one, so no count of failures is ever reported without saying
	// where it came from.
	Attribution Attribution       `json:"attribution"`
	Errors      []AttributedError `json:"errors,omitempty"`

	Labels []LabelSummary `json:"labels,omitempty"`
}

// Build summarises a run.
func Build(in Input) Report {
	rep := Report{
		ExecutionID: in.ExecutionID,
		ScenarioID:  in.ScenarioID,
		RunID:       in.RunID,
		Engine:      in.Engine,
		StartedAt:   in.StartedAt,
		EndedAt:     in.EndedAt,
		Outcome:     in.Outcome,
		Requested:   in.Requested,
	}

	overall := metrics.Histogram{}
	perLabel := map[string]*LabelSummary{}
	labelHist := map[string]metrics.Histogram{}
	peakConcurrency := 0

	for _, iv := range in.Intervals {
		if iv.Label == TotalLabel {
			// The engine's own aggregate; its requests are already counted.
			continue
		}
		rep.Achieved.Samples += iv.Samples
		rep.Achieved.Failed += iv.Failed
		overall.Merge(iv.Latency)
		if iv.Concurrency > peakConcurrency {
			peakConcurrency = iv.Concurrency
		}

		sum, ok := perLabel[iv.Label]
		if !ok {
			sum = &LabelSummary{Label: iv.Label}
			perLabel[iv.Label] = sum
			labelHist[iv.Label] = metrics.Histogram{}
		}
		sum.Samples += iv.Samples
		sum.Failed += iv.Failed
		labelHist[iv.Label].Merge(iv.Latency)
	}

	rep.Achieved.Concurrency = peakConcurrency
	rep.Achieved.DurationSeconds = in.Requested.DurationSeconds
	rep.Achieved.Throughput = perSecond(rep.Achieved.Samples, in.Requested.DurationSeconds)
	rep.ErrorRate = rate(rep.Achieved.Failed, rep.Achieved.Samples)
	rep.Latency = overall.Percentiles(reportedPercentiles...)
	rep.Errors, rep.Attribution = collectErrors(in.Intervals)

	names := make([]string, 0, len(perLabel))
	for name := range perLabel {
		names = append(names, name)
	}
	sort.Strings(names) // stable order, so two reports of the same run match
	for _, name := range names {
		sum := perLabel[name]
		sum.ErrorRate = rate(sum.Failed, sum.Samples)
		sum.Latency = labelHist[name].Percentiles(reportedPercentiles...)
		rep.Labels = append(rep.Labels, *sum)
	}
	return rep
}

// ShortOfRequest reports whether the run produced materially less load than it
// was asked for.
//
// It matters because latency measured under a fraction of the intended load says
// little about how the target behaves under the real thing -- a reader who
// misses that would draw a confident conclusion from a run that never happened
// as designed.
func (r Report) ShortOfRequest() bool {
	if r.Requested.Throughput <= 0 {
		return false // unlimited: there is no target rate to fall short of
	}
	const tolerance = 0.95
	return r.Achieved.Throughput < r.Requested.Throughput*tolerance
}

// Validate checks a report can be stored and read back meaningfully.
func (r Report) Validate() error {
	switch {
	case r.ExecutionID <= 0:
		return ErrExecutionRequired
	case r.RunID <= 0:
		return ErrRunRequired
	}
	switch r.Outcome {
	case taurus.OutcomePassed, taurus.OutcomeFailed, taurus.OutcomeAborted, taurus.OutcomeError:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrOutcomeUnknown, r.Outcome)
	}
}

func rate(part, whole int64) float64 {
	if whole <= 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

func perSecond(samples int64, seconds int) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(samples) / float64(seconds)
}
