// Package run holds the pure lifecycle domain of a execution's test run: the
// deployment phase, run identity, the legal state transitions between them, and
// the virtual-user usage calculation. No I/O.
package run

import (
	"errors"
	"time"

	"github.com/heridotlife/honryu/internal/domain/loadprofile"
)

// Phase is the lifecycle phase of a execution's engines.
type Phase string

const (
	// PhaseIdle means no engines are deployed.
	PhaseIdle Phase = "idle"
	// PhaseDeployed means engines are deployed but no run is in progress.
	PhaseDeployed Phase = "deployed"
	// PhaseRunning means a run is in progress.
	PhaseRunning Phase = "running"
)

// Lifecycle transition errors. Callers compare with errors.Is.
var (
	ErrNoScenarios     = errors.New("run: execution has no load profile entries")
	ErrNotDeployed     = errors.New("run: engines are not deployed")
	ErrEnginesNotReady = errors.New("run: not all engines are ready")
	ErrAlreadyRunning  = errors.New("run: a run is already in progress")
	ErrNotRunning      = errors.New("run: no run is in progress")
	// ErrEnginesFinished means the deployed engines already ran and finished
	// (their orphaned Finals arrived with no run open) -- a pod's bzt never
	// reruns on its own, so Trigger refuses to open a run nothing can feed;
	// the caller must Deploy fresh engines first.
	ErrEnginesFinished = errors.New("run: engines already finished, redeploy before triggering")
)

// Run is an in-progress or completed test run of an execution.
type Run struct {
	ID          int64
	ExecutionID int64
	StartedAt   time.Time
	EndedAt     *time.Time
}

// Finished reports whether the run has ended.
func (r Run) Finished() bool { return r.EndedAt != nil }

// Elapsed is how long the run lasted, or has lasted so far if unfinished,
// measured against now.
func (r Run) Elapsed(now time.Time) time.Duration {
	if r.EndedAt != nil {
		return r.EndedAt.Sub(r.StartedAt)
	}
	return now.Sub(r.StartedAt)
}

// DerivePhase computes the lifecycle phase from deployment and run facts.
func DerivePhase(enginesDeployed int, running bool) Phase {
	switch {
	case running:
		return PhaseRunning
	case enginesDeployed > 0:
		return PhaseDeployed
	default:
		return PhaseIdle
	}
}

// CanDeploy reports whether engines may be (re)deployed from the given phase.
// Deploying is idempotent from idle or deployed, but rejected mid-run.
func CanDeploy(phase Phase) error {
	if phase == PhaseRunning {
		return ErrAlreadyRunning
	}
	return nil
}

// CanTrigger validates a trigger request: at least one scenario must exist, engines
// must be deployed and fully ready, and no run may already be in progress.
func CanTrigger(phase Phase, ec loadprofile.Profile, enginesReady int) error {
	if len(ec.Tests) == 0 {
		return ErrNoScenarios
	}
	switch phase {
	case PhaseIdle:
		return ErrNotDeployed
	case PhaseRunning:
		return ErrAlreadyRunning
	}
	if enginesReady < ec.TotalEngines() {
		return ErrEnginesNotReady
	}
	return nil
}

// CanStop validates a stop request: a run must be in progress.
func CanStop(phase Phase) error {
	if phase != PhaseRunning {
		return ErrNotRunning
	}
	return nil
}

// VirtualUsers is the total concurrent virtual users an execution drives: the
// sum over scenarios of engines * concurrency. Used for usage accounting.
func VirtualUsers(ec loadprofile.Profile) int {
	vu := 0
	for _, ep := range ec.Tests {
		vu += ep.Engines * ep.Concurrency
	}
	return vu
}
