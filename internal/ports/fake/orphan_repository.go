package fake

import (
	"context"
	"sort"

	"github.com/heridotlife/honryu/internal/ports"
)

// Orphan completions, keyed by execution -> scenario/shard. A shard's Final is
// one event no matter how many times its sidecar retries, so re-recording the
// same shard overwrites. Guarded by Store's own mu (see repository.go).
type orphanKey struct {
	scenarioID int64
	shard      int
}

// RecordOrphanCompletion stores an orphaned shard completion, overwriting an
// earlier record of the same shard (a retried Final is one event).
func (s *Store) RecordOrphanCompletion(_ context.Context, oc ports.OrphanCompletion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.orphans == nil {
		s.orphans = map[int64]map[orphanKey]ports.OrphanCompletion{}
	}
	byShard, ok := s.orphans[oc.ExecutionID]
	if !ok {
		byShard = map[orphanKey]ports.OrphanCompletion{}
		s.orphans[oc.ExecutionID] = byShard
	}
	byShard[orphanKey{scenarioID: oc.ScenarioID, shard: oc.ShardIndex}] = oc
	return nil
}

// OrphanCompletions lists an execution's orphaned completions, stable order.
func (s *Store) OrphanCompletions(_ context.Context, executionID int64) ([]ports.OrphanCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	byShard := s.orphans[executionID]
	out := make([]ports.OrphanCompletion, 0, len(byShard))
	for _, oc := range byShard {
		out = append(out, oc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScenarioID != out[j].ScenarioID {
			return out[i].ScenarioID < out[j].ScenarioID
		}
		return out[i].ShardIndex < out[j].ShardIndex
	})
	return out, nil
}

// ClearOrphanCompletions drops every orphan row for an execution.
func (s *Store) ClearOrphanCompletions(_ context.Context, executionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.orphans, executionID)
	return nil
}
