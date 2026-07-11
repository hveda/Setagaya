package fake

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// StartRun creates the active run for a collection, opening a history row.
func (s *Store) StartRun(_ context.Context, collectionID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.currentRun[collectionID]; ok {
		return 0, ports.ErrRunActive
	}
	s.runSeq++
	runID := s.runSeq
	s.currentRun[collectionID] = runID
	s.runHistory[runID] = &ports.RunRecord{RunID: runID, CollectionID: collectionID, StartedTime: s.now()}
	return runID, nil
}

// CurrentRun returns the active run id for a collection.
func (s *Store) CurrentRun(_ context.Context, collectionID int64) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, ok := s.currentRun[collectionID]
	return runID, ok, nil
}

// StopRun clears the active run and stamps its history end time.
func (s *Store) StopRun(_ context.Context, collectionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, ok := s.currentRun[collectionID]
	if !ok {
		return nil
	}
	if rec := s.runHistory[runID]; rec != nil && rec.EndTime == nil {
		end := s.now()
		rec.EndTime = &end
	}
	delete(s.currentRun, collectionID)
	return nil
}

// MarkPlanRunning records a running plan (idempotent).
func (s *Store) MarkPlanRunning(_ context.Context, collectionID, planID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plans, ok := s.running[collectionID]
	if !ok {
		plans = map[int64]time.Time{}
		s.running[collectionID] = plans
	}
	if _, exists := plans[planID]; !exists {
		plans[planID] = s.now()
	}
	return nil
}

// ClearPlanRunning removes a running plan marker (idempotent).
func (s *Store) ClearPlanRunning(_ context.Context, collectionID, planID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running[collectionID], planID)
	if len(s.running[collectionID]) == 0 {
		delete(s.running, collectionID)
	}
	return nil
}

// RunningPlans lists every running plan across collections.
func (s *Store) RunningPlans(_ context.Context) ([]ports.RunningPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningPlan
	for collectionID, plans := range s.running {
		for planID, started := range plans {
			out = append(out, ports.RunningPlan{CollectionID: collectionID, PlanID: planID, StartedTime: started})
		}
	}
	sortRunningPlans(out)
	return out, nil
}

// RunningPlansByCollection lists running plans for one collection.
func (s *Store) RunningPlansByCollection(_ context.Context, collectionID int64) ([]ports.RunningPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ports.RunningPlan
	for planID, started := range s.running[collectionID] {
		out = append(out, ports.RunningPlan{CollectionID: collectionID, PlanID: planID, StartedTime: started})
	}
	sortRunningPlans(out)
	return out, nil
}

func sortRunningPlans(rps []ports.RunningPlan) {
	sort.Slice(rps, func(i, j int) bool {
		if rps[i].CollectionID != rps[j].CollectionID {
			return rps[i].CollectionID < rps[j].CollectionID
		}
		return rps[i].PlanID < rps[j].PlanID
	})
}
