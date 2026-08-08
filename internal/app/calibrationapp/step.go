package calibrationapp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/report"
)

// ErrScenarioNotConfigured means executionID's load profile does not name
// exactly one scenario -- a calibration execution is bound to its single
// target the ordinary way (an executionapp.StoreConfig upload) before its
// first job is ever triggered, and RunStep only ever rewrites that one
// entry's QPS-varying fields, never which scenario it targets.
var ErrScenarioNotConfigured = errors.New("calibrationapp: execution has no single scenario configured")

// ErrRequestedQPSInvalid means requestedQPS was not positive. Zero has a
// distinct, dangerous meaning to loadprofile.Entry.Throughput ("unlimited"),
// so a non-positive request is rejected rather than silently reinterpreted.
var ErrRequestedQPSInvalid = errors.New("calibrationapp: requested QPS must be positive")

// ErrStepRunNotStarted means Trigger returned without error but the
// execution has no current run to hold and measure -- should not happen in
// practice, but a step must not silently read an unrelated run's report.
var ErrStepRunNotStarted = errors.New("calibrationapp: triggered run is not reported as running")

const (
	// stepConcurrencyFloor is the least concurrency ever requested, so even
	// the search's seed QPS keeps enough virtual users to actually reach it.
	stepConcurrencyFloor = 20
	// stepConcurrencyPerQPS is how many virtual users a step reserves per
	// unit of requested QPS -- generous (assumes up to ~2s worst-case
	// response time, Little's Law) so Taurus's own thread ceiling is never
	// what limits throughput; only the pod's real capacity or the target's
	// real behaviour is allowed to.
	stepConcurrencyPerQPS = 2.0
	// stepRampupSeconds is the warmup a step's run spends ramping to its
	// full concurrency before the steady-state hold begins.
	stepRampupSeconds = 5
)

// RunnerRepo is the persistence a step needs beyond driving the run itself:
// reading and rewriting the calibration execution's own load profile, and
// finding the run a triggered step produced.
type RunnerRepo interface {
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
	StoreLoadProfile(ctx context.Context, executionID int64, csvSplit bool, entries []loadprofile.Entry) error
	CurrentRun(ctx context.Context, executionID int64) (runID int64, ok bool, err error)
}

// Lifecycle is the ordinary test-lifecycle mechanism a step drives -- reused
// rather than reinvented, so a calibration step inherits campaign-freeze
// gating and engine-equivalents quota exactly as any other trigger does.
// *lifecycleapp.Service satisfies this directly.
type Lifecycle interface {
	Deploy(ctx context.Context, executionID int64) error
	Trigger(ctx context.Context, executionID int64) error
	Stop(ctx context.Context, executionID int64) error
}

// ReportReader reads back a run's settled report once it has ended.
// ports.ReportStore satisfies this directly.
type ReportReader interface {
	GetReport(ctx context.Context, runID int64) (report.Report, error)
}

// StepRunner drives a single calibration step: a real, settled, one-pod run
// at a chosen QPS. Each step is its own genuine run through the ordinary
// lifecycle (not a synthetic estimate) so its settled report's
// requested-vs-achieved gap and target-health signals are trustworthy.
type StepRunner struct {
	repo      RunnerRepo
	lifecycle Lifecycle
	reports   ReportReader
	sleep     func(time.Duration)
}

// NewStepRunner wires a step runner. sleep defaults to time.Sleep; override
// with WithSleep for deterministic tests.
func NewStepRunner(repo RunnerRepo, lifecycle Lifecycle, reports ReportReader) *StepRunner {
	return &StepRunner{repo: repo, lifecycle: lifecycle, reports: reports, sleep: time.Sleep}
}

// WithSleep overrides the hold mechanism. Returns the receiver for chaining.
func (r *StepRunner) WithSleep(sleep func(time.Duration)) *StepRunner {
	if sleep != nil {
		r.sleep = sleep
	}
	return r
}

// RunStep rewrites executionID's load profile to a single pinned-size pod
// running at requestedQPS, deploys and triggers it, holds through the
// steady-state window, stops it, and returns the settled report.
//
// executionID's pod size is not RunStep's concern -- it is pinned on the
// execution itself (execution.CPU/Memory, set once at calibrationapp.Create)
// and Deploy already reads it from there for every execution, calibration or
// not.
func (r *StepRunner) RunStep(ctx context.Context, executionID int64, requestedQPS float64, holdSeconds int) (report.Report, error) {
	if requestedQPS <= 0 {
		return report.Report{}, fmt.Errorf("%w: %g", ErrRequestedQPSInvalid, requestedQPS)
	}

	entries, err := r.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return report.Report{}, err
	}
	if len(entries) != 1 {
		return report.Report{}, fmt.Errorf("%w: execution %d has %d scenarios, want exactly 1", ErrScenarioNotConfigured, executionID, len(entries))
	}
	base := entries[0]

	step := loadprofile.Entry{
		Name:        base.Name,
		ScenarioID:  base.ScenarioID,
		Engines:     1,
		Concurrency: stepConcurrency(requestedQPS),
		Rampup:      stepRampupSeconds,
		Duration:    holdSeconds,
		Throughput:  int(math.Ceil(requestedQPS)),
	}
	if err := step.Validate(); err != nil {
		return report.Report{}, err
	}
	// CSVSplit never matters here: a single-engine step never splits data
	// across engines.
	if err := r.repo.StoreLoadProfile(ctx, executionID, false, []loadprofile.Entry{step}); err != nil {
		return report.Report{}, err
	}

	if err := r.lifecycle.Deploy(ctx, executionID); err != nil {
		return report.Report{}, err
	}
	if err := r.lifecycle.Trigger(ctx, executionID); err != nil {
		return report.Report{}, err
	}
	runID, running, err := r.repo.CurrentRun(ctx, executionID)
	if err != nil {
		return report.Report{}, err
	}
	if !running {
		return report.Report{}, fmt.Errorf("%w: execution %d", ErrStepRunNotStarted, executionID)
	}

	r.sleep(time.Duration(stepRampupSeconds+holdSeconds) * time.Second)

	if err := r.lifecycle.Stop(ctx, executionID); err != nil {
		return report.Report{}, err
	}
	return r.reports.GetReport(ctx, runID)
}

// stepConcurrency returns the virtual-user count a step at requestedQPS
// should run with -- generous enough that concurrency itself is never the
// bottleneck a classification mistakes for engine or target saturation.
func stepConcurrency(requestedQPS float64) int {
	c := int(math.Ceil(requestedQPS * stepConcurrencyPerQPS))
	if c < stepConcurrencyFloor {
		return stepConcurrencyFloor
	}
	return c
}
