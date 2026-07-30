package fake

import (
	"context"
	"time"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// StartLaunch opens a launch for an execution.
func (s *Store) StartLaunch(_ context.Context, executionID int64, owner string, engines, vu int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.openLaunch[executionID]; ok {
		return ports.ErrLaunchActive
	}
	rec := &ports.LaunchRecord{
		ExecutionID: executionID,
		Context:     s.deployContext,
		Owner:       owner,
		Engines:     engines,
		VU:          vu,
		StartedTime: s.now(),
	}
	s.openLaunch[executionID] = rec
	s.launchHistory = append(s.launchHistory, rec)
	return nil
}

// FinishLaunch stamps the open launch's end time and final VU.
func (s *Store) FinishLaunch(_ context.Context, executionID int64, vu int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.openLaunch[executionID]
	if !ok {
		return nil
	}
	end := s.now()
	rec.EndTime = &end
	rec.VU = vu
	delete(s.openLaunch, executionID)
	return nil
}

// LaunchHistory returns finished launches within [from, to].
func (s *Store) LaunchHistory(_ context.Context, from, to time.Time) ([]ports.LaunchRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.LaunchRecord
	for _, rec := range s.launchHistory {
		if rec.EndTime == nil {
			continue
		}
		if rec.StartedTime.Before(from) || rec.EndTime.After(to) {
			continue
		}
		out = append(out, *rec)
	}
	return out, nil
}
