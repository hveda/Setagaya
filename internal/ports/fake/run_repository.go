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

// MarkScenarioRunning records a running plan (idempotent).
func (s *Store) MarkScenarioRunning(_ context.Context, executionID, scenarioID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plans, ok := s.running[executionID]
	if !ok {
		plans = map[int64]time.Time{}
		s.running[executionID] = plans
	}
	if _, exists := plans[scenarioID]; !exists {
		plans[scenarioID] = s.now()
	}
	return nil
}

// ClearScenarioRunning removes a running plan marker (idempotent).
func (s *Store) ClearScenarioRunning(_ context.Context, executionID, scenarioID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running[executionID], scenarioID)
	if len(s.running[executionID]) == 0 {
		delete(s.running, executionID)
	}
	return nil
}

// RunningScenarios lists every running plan across collections.
func (s *Store) RunningScenarios(_ context.Context) ([]ports.RunningScenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningScenario
	for executionID, plans := range s.running {
		for scenarioID, started := range plans {
			out = append(out, ports.RunningScenario{ExecutionID: executionID, ScenarioID: scenarioID, StartedTime: started})
		}
	}
	sortRunningPlans(out)
	return out, nil
}

// RunningScenariosByExecution lists running plans for one collection.
func (s *Store) RunningScenariosByExecution(_ context.Context, executionID int64) ([]ports.RunningScenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningScenario
	for scenarioID, started := range s.running[executionID] {
		out = append(out, ports.RunningScenario{ExecutionID: executionID, ScenarioID: scenarioID, StartedTime: started})
	}
	sortRunningPlans(out)
	return out, nil
}

func sortRunningPlans(rps []ports.RunningScenario) {
	sort.Slice(rps, func(i, j int) bool {
		if rps[i].ExecutionID != rps[j].ExecutionID {
			return rps[i].ExecutionID < rps[j].ExecutionID
		}
		return rps[i].ScenarioID < rps[j].ScenarioID
	})
}
