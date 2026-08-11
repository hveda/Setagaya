// Package schedule models a time-triggered execution: fire once at a
// specific instant, or recurring on a cron expression. Pure domain: no I/O,
// no persistence -- Occurrences computes fire times within a window, nothing
// else. The actual firing (deploy + trigger at the computed instant) lives in
// cmd/scheduler, well outside this package.
package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// Kind distinguishes a one-shot schedule from a recurring one.
type Kind string

const (
	// KindOneShot fires exactly once, at FireAt.
	KindOneShot Kind = "one_shot"
	// KindRecurring fires on every occurrence of its cron Recurrence.
	KindRecurring Kind = "recurring"
)

// Validation errors. Callers compare with errors.Is.
var (
	ErrExecutionRequired  = errors.New("schedule: a valid execution id is required")
	ErrKindInvalid        = errors.New("schedule: kind must be one_shot or recurring")
	ErrFireAtRequired     = errors.New("schedule: a one_shot schedule requires fire_at")
	ErrRecurrenceRequired = errors.New("schedule: a recurring schedule requires a recurrence expression")
	ErrRecurrenceInvalid  = errors.New("schedule: recurrence is not a valid cron expression")
	ErrWindowInvalid      = errors.New("schedule: window end must be after start")
)

// Schedule is a request to run an already-configured execution at a future
// time, once or on a recurrence.
//
// A schedule has no cluster of its own: the cluster a scheduled run deploys to
// -- and reserves quota against -- is the execution's (execution.Execution.Cluster),
// the single source of truth (Phase 8). This also removed an inconsistency
// where a schedule stored a cluster for quota but the fire path deployed to the
// control plane's own cluster regardless.
type Schedule struct {
	ID          int64
	ExecutionID int64
	TenantID    int64
	Kind        Kind
	// FireAt is the single fire time for a KindOneShot schedule; unused (and
	// typically nil) for KindRecurring.
	FireAt *time.Time
	// Recurrence is a standard 5-field cron expression for a KindRecurring
	// schedule; unused for KindOneShot.
	Recurrence string
	Active     bool
}

// Validate checks a schedule's own invariants, independent of quota or
// persistence.
func (s Schedule) Validate() error {
	if s.ExecutionID <= 0 {
		return ErrExecutionRequired
	}
	switch s.Kind {
	case KindOneShot:
		if s.FireAt == nil {
			return ErrFireAtRequired
		}
		return nil
	case KindRecurring:
		if s.Recurrence == "" {
			return ErrRecurrenceRequired
		}
		if _, err := cron.ParseStandard(s.Recurrence); err != nil {
			return fmt.Errorf("%w: %v", ErrRecurrenceInvalid, err)
		}
		return nil
	default:
		return ErrKindInvalid
	}
}

// Occurrences computes every instant within the half-open window [from, to)
// at which s fires: a one-shot schedule contributes its single FireAt if
// inside the window; a recurring schedule steps its cron expression forward
// from the window's start until it walks past the window's end.
func (s Schedule) Occurrences(from, to time.Time) ([]time.Time, error) {
	if !to.After(from) {
		return nil, ErrWindowInvalid
	}
	switch s.Kind {
	case KindOneShot:
		if s.FireAt == nil {
			return nil, ErrFireAtRequired
		}
		if !s.FireAt.Before(from) && s.FireAt.Before(to) {
			return []time.Time{*s.FireAt}, nil
		}
		return nil, nil
	case KindRecurring:
		sched, err := cron.ParseStandard(s.Recurrence)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRecurrenceInvalid, err)
		}
		var out []time.Time
		// cron.Schedule.Next(t) returns the first fire time strictly after t;
		// seeding one nanosecond before the window lets a fire time landing
		// exactly on `from` still be included.
		t := from.Add(-time.Nanosecond)
		for {
			next := sched.Next(t)
			if next.IsZero() || !next.Before(to) {
				break
			}
			out = append(out, next)
			t = next
		}
		return out, nil
	default:
		return nil, ErrKindInvalid
	}
}
