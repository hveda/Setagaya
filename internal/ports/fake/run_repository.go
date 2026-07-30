package fake

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// StartRun creates the active run for an execution, opening a history row.
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

// CurrentRun returns the active run id for an execution.
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

// MarkScenarioRunning records a running scenario (idempotent).
func (s *Store) MarkScenarioRunning(_ context.Context, executionID, scenarioID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scenarios, ok := s.running[executionID]
	if !ok {
		scenarios = map[int64]time.Time{}
		s.running[executionID] = scenarios
	}
	if _, exists := scenarios[scenarioID]; !exists {
		scenarios[scenarioID] = s.now()
	}
	return nil
}

// ClearScenarioRunning removes a running scenario marker (idempotent).
func (s *Store) ClearScenarioRunning(_ context.Context, executionID, scenarioID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running[executionID], scenarioID)
	if len(s.running[executionID]) == 0 {
		delete(s.running, executionID)
	}
	return nil
}

// RunningScenarios lists every running scenario across executions.
func (s *Store) RunningScenarios(_ context.Context) ([]ports.RunningScenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningScenario
	for executionID, scenarios := range s.running {
		for scenarioID, started := range scenarios {
			out = append(out, ports.RunningScenario{ExecutionID: executionID, ScenarioID: scenarioID, StartedTime: started})
		}
	}
	sortRunningScenarios(out)
	return out, nil
}

// RunningScenariosByExecution lists running scenarios for one execution.
func (s *Store) RunningScenariosByExecution(_ context.Context, executionID int64) ([]ports.RunningScenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningScenario
	for scenarioID, started := range s.running[executionID] {
		out = append(out, ports.RunningScenario{ExecutionID: executionID, ScenarioID: scenarioID, StartedTime: started})
	}
	sortRunningScenarios(out)
	return out, nil
}

func sortRunningScenarios(rps []ports.RunningScenario) {
	sort.Slice(rps, func(i, j int) bool {
		if rps[i].ExecutionID != rps[j].ExecutionID {
			return rps[i].ExecutionID < rps[j].ExecutionID
		}
		return rps[i].ScenarioID < rps[j].ScenarioID
	})
}
