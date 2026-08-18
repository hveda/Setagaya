package ports

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/capacityprofile"
)

// CapacityProfileRepository persists calibration outcomes, keyed by
// capacityprofile.Key.
type CapacityProfileRepository interface {
	// UpsertCapacityProfile replaces whatever profile exists for
	// profile.Key with profile -- a later calibration always supersedes an
	// earlier one for the same (scenario, engine, cpu, memory).
	UpsertCapacityProfile(ctx context.Context, profile capacityprofile.CapacityProfile) error
	// GetCapacityProfile returns the profile for key, or ErrNotFound.
	GetCapacityProfile(ctx context.Context, key capacityprofile.Key) (capacityprofile.CapacityProfile, error)
	// ListCapacityProfiles returns every stored profile.
	ListCapacityProfiles(ctx context.Context) ([]capacityprofile.CapacityProfile, error)
}
