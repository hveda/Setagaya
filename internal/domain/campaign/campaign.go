// Package campaign models a PM-owned readiness event: a window, the
// services participating in it, and each one's designated readiness
// execution. Pure domain: no I/O, no persistence.
//
// Campaign holds no execution semantics of its own -- it is a pure
// coordination layer above executions that already exist. Its window
// determines when freeze applies and which of its services' runs count
// toward the rolled-up verdict; the verdict itself is computed elsewhere
// from each designated execution's own report.
package campaign

import (
	"errors"
	"time"
)

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired            = errors.New("campaign: name is required")
	ErrWindowInvalid           = errors.New("campaign: window end must be after start")
	ErrServicesRequired        = errors.New("campaign: at least one participating service is required")
	ErrDuplicateService        = errors.New("campaign: a project may participate at most once")
	ErrProjectRequired         = errors.New("campaign: a valid project id is required")
	ErrServiceExecutionInvalid = errors.New("campaign: a valid designated execution id is required")
)

// Window is the campaign's half-open readiness period [Start, End).
type Window struct {
	Start time.Time
	End   time.Time
}

// Service is one participating service (Project) and the execution
// its owner designated as that service's readiness test -- the only
// execution under that project freeze exempts, and the one whose report
// decides that service's verdict.
type Service struct {
	ProjectID   int64
	ExecutionID int64
}

// Campaign is a PM-owned readiness event: a window, participating services,
// and (once ended or aborted) a rolled-up verdict computed elsewhere.
//
// TenantID is a plain int64, not ports.ClusterRef-style indirection: domain
// packages do not import ports, and every participating project already
// belongs to this same tenant (enforced by campaignapp, not here -- this
// type only knows about its own fields).
//
// AbortedAt is nil for a campaign that has not been aborted. There is no
// separate stored status: IsActive derives activity from Window and
// AbortedAt together, the same way run.DerivePhase derives a run's phase
// from raw facts rather than a redundant stored enum.
type Campaign struct {
	ID        int64
	Name      string
	TenantID  int64
	Window    Window
	Services  []Service
	AbortedAt *time.Time
}

// Validate checks a campaign's own invariants, independent of anything else
// already scheduled or running.
func (c Campaign) Validate() error {
	switch {
	case c.Name == "":
		return ErrNameRequired
	case !c.Window.End.After(c.Window.Start):
		return ErrWindowInvalid
	case len(c.Services) == 0:
		return ErrServicesRequired
	}
	seen := make(map[int64]struct{}, len(c.Services))
	for _, svc := range c.Services {
		if svc.ProjectID <= 0 {
			return ErrProjectRequired
		}
		if svc.ExecutionID <= 0 {
			return ErrServiceExecutionInvalid
		}
		if _, dup := seen[svc.ProjectID]; dup {
			return ErrDuplicateService
		}
		seen[svc.ProjectID] = struct{}{}
	}
	return nil
}

// IsActive reports whether the campaign is currently in force at now: its
// window contains now (half-open, [Start, End)) and it has not been
// aborted. A frozen/blocked check (lifecycleapp's Freeze hook) and the
// drain sweep both key off this, not a separately-maintained flag.
func (c Campaign) IsActive(now time.Time) bool {
	if c.AbortedAt != nil {
		return false
	}
	return !now.Before(c.Window.Start) && now.Before(c.Window.End)
}

// DesignatedExecution reports the execution registered as projectID's
// readiness test, if projectID participates in the campaign at all.
func (c Campaign) DesignatedExecution(projectID int64) (executionID int64, ok bool) {
	for _, svc := range c.Services {
		if svc.ProjectID == projectID {
			return svc.ExecutionID, true
		}
	}
	return 0, false
}
