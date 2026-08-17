package adminapp

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
)

// The default CampaignScoper (used when a deployment wires no campaigns) must
// report "not found" for every method: kill-switch campaign scoping is
// unreachable there and must match nothing rather than fail open.
func TestNoopCampaignScoper_MatchesNothing(t *testing.T) {
	t.Parallel()
	s := noopCampaignScoper{}
	ctx := context.Background()

	if _, err := s.Get(ctx, 1); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Get = %v, want ErrNotFound", err)
	}
	if ids, err := s.InScopeExecutions(ctx, 1); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("InScopeExecutions = %v, %v, want ErrNotFound", ids, err)
	}
	if err := s.Abort(ctx, 1); !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Abort = %v, want ErrNotFound", err)
	}
}
