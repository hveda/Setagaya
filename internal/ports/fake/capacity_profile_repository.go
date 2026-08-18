package fake

import (
	"context"
	"sort"

	"github.com/heridotlife/honryu/internal/domain/capacityprofile"
	"github.com/heridotlife/honryu/internal/ports"
)

// UpsertCapacityProfile replaces whatever profile exists for profile.Key.
func (s *Store) UpsertCapacityProfile(_ context.Context, profile capacityprofile.CapacityProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capacityProfiles[profile.Key] = profile
	return nil
}

// GetCapacityProfile returns the profile for key, or ports.ErrNotFound.
func (s *Store) GetCapacityProfile(_ context.Context, key capacityprofile.Key) (capacityprofile.CapacityProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.capacityProfiles[key]
	if !ok {
		return capacityprofile.CapacityProfile{}, ports.ErrNotFound
	}
	return p, nil
}

// ListCapacityProfiles returns every stored profile, in a stable order.
func (s *Store) ListCapacityProfiles(_ context.Context) ([]capacityprofile.CapacityProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]capacityprofile.CapacityProfile, 0, len(s.capacityProfiles))
	for _, p := range s.capacityProfiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Key, out[j].Key
		if a.ScenarioID != b.ScenarioID {
			return a.ScenarioID < b.ScenarioID
		}
		if a.Engine != b.Engine {
			return a.Engine < b.Engine
		}
		if a.CPU != b.CPU {
			return a.CPU < b.CPU
		}
		return a.Memory < b.Memory
	})
	return out, nil
}
