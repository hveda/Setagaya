package ports

import (
	"context"
	"time"

	"github.com/heridotlife/honryu/internal/domain/schedule"
)

// OccurrenceStatus is the outcome of one schedule's fire time, decided when
// it was reserved -- at schedule creation, or a later horizon extension for a
// recurring schedule -- not deferred until the moment it actually fires.
type OccurrenceStatus string

const (
	// OccurrenceReserved holds capacity for the occurrence; cmd/scheduler
	// deploys and triggers it when its fire time comes.
	OccurrenceReserved OccurrenceStatus = "reserved"
	// OccurrenceRejected means the occurrence did not fit the tenant's quota
	// when it was reserved -- visible to the schedule's owner, never silent.
	OccurrenceRejected OccurrenceStatus = "rejected"
	// OccurrenceFired means cmd/scheduler has deployed and triggered it.
	OccurrenceFired OccurrenceStatus = "fired"
	// OccurrenceCompleted means the run this occurrence started has ended.
	OccurrenceCompleted OccurrenceStatus = "completed"
)

// Occurrence is one computed fire time for a schedule and its outcome.
// ReservationID is set only while OccurrenceReserved -- once fired or
// rejected, the reservation it named (if any) has already been consumed or
// never existed.
type Occurrence struct {
	ID            int64
	ScheduleID    int64
	FireTime      time.Time
	Status        OccurrenceStatus
	ReservationID *int64
}

// ScheduleRepository persists schedules and the occurrences their creation
// (or a later horizon extension) computed and reserved.
type ScheduleRepository interface {
	// CreateSchedule persists s and returns its assigned ID.
	CreateSchedule(ctx context.Context, s schedule.Schedule) (int64, error)
	// GetSchedule returns the schedule with id, or ErrNotFound.
	GetSchedule(ctx context.Context, id int64) (schedule.Schedule, error)
	// ListSchedulesByExecution returns every schedule belonging to executionID.
	ListSchedulesByExecution(ctx context.Context, executionID int64) ([]schedule.Schedule, error)
	// ListActiveRecurringSchedules returns every active recurring schedule
	// across all executions -- what the horizon-extension pass rolls forward.
	// One-shot schedules are never returned: they have exactly one occurrence,
	// computed once at creation, with no horizon to extend.
	ListActiveRecurringSchedules(ctx context.Context) ([]schedule.Schedule, error)
	// DeleteSchedule removes a schedule and every occurrence it owns, or
	// ErrNotFound. Deleting a schedule does not itself release any
	// still-reserved occurrence's reservation -- the caller (scheduleapp)
	// does that first, while it can still see which occurrences hold one.
	DeleteSchedule(ctx context.Context, id int64) error

	// CreateOccurrence persists o and returns its assigned ID.
	CreateOccurrence(ctx context.Context, o Occurrence) (int64, error)
	// OccurrencesForSchedule returns every occurrence belonging to
	// scheduleID, ordered by fire time.
	OccurrencesForSchedule(ctx context.Context, scheduleID int64) ([]Occurrence, error)

	// ClaimDueOccurrence atomically finds the earliest still-reserved
	// occurrence whose fire time is at or before now, marks it fired, and
	// returns it -- the exclusive hand-off that lets more than one
	// cmd/scheduler replica poll concurrently without two of them firing the
	// same occurrence. found is false when nothing is due. Implementations
	// must make the find-and-mark step atomic (row-locking in MySQL; the
	// fake's single mutex gives the same guarantee for free).
	ClaimDueOccurrence(ctx context.Context, now time.Time) (o Occurrence, found bool, err error)

	// RecordHorizonExtension records that the recurring-schedule
	// horizon-extension pass completed at t, overwriting whatever was
	// recorded before -- a single timestamp queryable to tell whether the
	// background job is stalled, rather than silently leaving future
	// occurrences unguarded.
	RecordHorizonExtension(ctx context.Context, t time.Time) error
	// LastHorizonExtension returns the last successful horizon-extension
	// pass's timestamp, or found=false if one has never completed.
	LastHorizonExtension(ctx context.Context) (t time.Time, found bool, err error)

	// WithScheduleLock runs fn while holding an exclusive lock scoped to
	// scheduleID, serializing occurrence-mutating operations for one
	// schedule -- specifically, horizon extension: without this, two
	// cmd/scheduler replicas extending the same schedule's horizon around
	// the same time could each read the same "existing occurrences" before
	// either commits its own new ones, creating duplicate occurrences (and
	// reservations) for the same fire time, which would later each be
	// claimed and fired independently. A call for a different scheduleID
	// must not block on this one. fn's error (if any) is returned
	// unchanged; the lock is always released, even when fn errors.
	WithScheduleLock(ctx context.Context, scheduleID int64, fn func(ctx context.Context) error) error
}
