package fake

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// StartRun creates the active run for a collection, opening a history row.
func (s *Store) StartRun(_ context.Context, executionID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.currentRun[executionID]; ok {
		return 0, ports.ErrRunActive
	}
	s.runSeq++
	runID := s.runSeq
	s.currentRun[executionID] = runID
	s.runHistory[runID] = &ports.RunRecord{RunID: runID, ExecutionID: executionID, StartedTime: s.now()}
	return runID, nil
}

// CurrentRun returns the active run id for a collection.
func (s *Store) CurrentRun(_ context.Context, executionID int64) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, ok := s.currentRun[executionID]
	return runID, ok, nil
}

// StopRun clears the active run and stamps its history end time.
func (s *Store) StopRun(_ context.Context, executionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, ok := s.currentRun[executionID]
	if !ok {
		return nil
	}
	if rec := s.runHistory[runID]; rec != nil && rec.EndTime == nil {
		end := s.now()
		rec.EndTime = &end
	}
	delete(s.currentRun, executionID)
	return nil
}

// MarkPlanRunning records a running plan (idempotent).
func (s *Store) MarkPlanRunning(_ context.Context, executionID, planID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plans, ok := s.running[executionID]
	if !ok {
		plans = map[int64]time.Time{}
		s.running[executionID] = plans
	}
	if _, exists := plans[planID]; !exists {
		plans[planID] = s.now()
	}
	return nil
}

// ClearPlanRunning removes a running plan marker (idempotent).
func (s *Store) ClearPlanRunning(_ context.Context, executionID, planID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running[executionID], planID)
	if len(s.running[executionID]) == 0 {
		delete(s.running, executionID)
	}
	return nil
}

// RunningPlans lists every running plan across collections.
func (s *Store) RunningPlans(_ context.Context) ([]ports.RunningPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningPlan
	for executionID, plans := range s.running {
		for planID, started := range plans {
			out = append(out, ports.RunningPlan{ExecutionID: executionID, PlanID: planID, StartedTime: started})
		}
	}
	sortRunningPlans(out)
	return out, nil
}

// RunningPlansByCollection lists running plans for one collection.
func (s *Store) RunningPlansByCollection(_ context.Context, executionID int64) ([]ports.RunningPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningPlan
	for planID, started := range s.running[executionID] {
		out = append(out, ports.RunningPlan{ExecutionID: executionID, PlanID: planID, StartedTime: started})
	}
	sortRunningPlans(out)
	return out, nil
}

func sortRunningPlans(rps []ports.RunningPlan) {
	sort.Slice(rps, func(i, j int) bool {
		if rps[i].ExecutionID != rps[j].ExecutionID {
			return rps[i].ExecutionID < rps[j].ExecutionID
		}
		return rps[i].PlanID < rps[j].PlanID
	})
}
