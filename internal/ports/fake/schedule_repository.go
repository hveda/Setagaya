package fake

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/ports"
)

// CreateSchedule persists s and returns its assigned ID.
func (s *Store) CreateSchedule(_ context.Context, sc schedule.Schedule) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleSeq++
	sc.ID = s.scheduleSeq
	s.schedules[sc.ID] = sc
	return sc.ID, nil
}

// GetSchedule returns the schedule with id, or ports.ErrNotFound.
func (s *Store) GetSchedule(_ context.Context, id int64) (schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.schedules[id]
	if !ok {
		return schedule.Schedule{}, ports.ErrNotFound
	}
	return sc, nil
}

// ListSchedulesByExecution returns every schedule belonging to executionID.
func (s *Store) ListSchedulesByExecution(_ context.Context, executionID int64) ([]schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []schedule.Schedule{}
	for _, sc := range s.schedules {
		if sc.ExecutionID == executionID {
			out = append(out, sc)
		}
	}
	return out, nil
}

// ListActiveRecurringSchedules returns every active recurring schedule
// across all executions.
func (s *Store) ListActiveRecurringSchedules(_ context.Context) ([]schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []schedule.Schedule{}
	for _, sc := range s.schedules {
		if sc.Kind == schedule.KindRecurring && sc.Active {
			out = append(out, sc)
		}
	}
	return out, nil
}

// DeleteSchedule removes a schedule and every occurrence it owns, or
// ports.ErrNotFound.
func (s *Store) DeleteSchedule(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.schedules[id]; !ok {
		return ports.ErrNotFound
	}
	delete(s.schedules, id)
	for occID, o := range s.occurrences {
		if o.ScheduleID == id {
			delete(s.occurrences, occID)
		}
	}
	return nil
}

// CreateOccurrence persists o and returns its assigned ID.
func (s *Store) CreateOccurrence(_ context.Context, o ports.Occurrence) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.occurrenceSeq++
	o.ID = s.occurrenceSeq
	s.occurrences[o.ID] = o
	return o.ID, nil
}

// OccurrencesForSchedule returns every occurrence belonging to scheduleID,
// ordered by fire time.
func (s *Store) OccurrencesForSchedule(_ context.Context, scheduleID int64) ([]ports.Occurrence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []ports.Occurrence{}
	for _, o := range s.occurrences {
		if o.ScheduleID == scheduleID {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FireTime.Before(out[j].FireTime) })
	return out, nil
}

// ClaimDueOccurrence finds the earliest still-reserved occurrence due at or
// before now and marks it fired. The Store's single mutex, held for the
// whole operation, gives the same exclusivity a real row lock would.
func (s *Store) ClaimDueOccurrence(_ context.Context, now time.Time) (ports.Occurrence, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var claimed ports.Occurrence
	found := false
	for _, o := range s.occurrences {
		if o.Status != ports.OccurrenceReserved || o.FireTime.After(now) {
			continue
		}
		if !found || o.FireTime.Before(claimed.FireTime) {
			claimed = o
			found = true
		}
	}
	if !found {
		return ports.Occurrence{}, false, nil
	}
	claimed.Status = ports.OccurrenceFired
	s.occurrences[claimed.ID] = claimed
	return claimed, true, nil
}

// RecordHorizonExtension records that the horizon-extension pass completed
// at t, overwriting whatever was recorded before.
func (s *Store) RecordHorizonExtension(_ context.Context, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.horizonRunAt = &t
	return nil
}

// LastHorizonExtension returns the last successful horizon-extension pass's
// timestamp, or found=false if one has never completed.
func (s *Store) LastHorizonExtension(_ context.Context) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.horizonRunAt == nil {
		return time.Time{}, false, nil
	}
	return *s.horizonRunAt, true, nil
}

// WithScheduleLock runs fn while holding an exclusive lock scoped to
// scheduleID, distinct from s.mu (fn calls back into this Store's own
// methods, which lock s.mu themselves).
func (s *Store) WithScheduleLock(ctx context.Context, scheduleID int64, fn func(context.Context) error) error {
	return s.withNamedLock(fmt.Sprintf("schedule:%d", scheduleID), func() error { return fn(ctx) })
}
