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
}
