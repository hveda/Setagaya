package fake

import (
	"context"
	"sort"

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
