package repositorytest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewCampaignRepo builds a fresh, empty CampaignRepository for one test.
type NewCampaignRepo func(t *testing.T) ports.CampaignRepository

func testCampaign(tenantID int64, start, end time.Time) campaign.Campaign {
	return campaign.Campaign{
		Name:     "Supersale 11.11",
		TenantID: tenantID,
		Window:   campaign.Window{Start: start, End: end},
		Services: []campaign.Service{
			{ProjectID: 1, ExecutionID: 10},
			{ProjectID: 2, ExecutionID: 20},
		},
	}
}

// RunCampaignRepositoryContract pins the behaviour every CampaignRepository
// must share.
func RunCampaignRepositoryContract(t *testing.T, newRepo NewCampaignRepo) {
	t.Helper()

	t.Run("CreateAndGetRoundTripsServices", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateCampaign(ctx, testCampaign(7, at(0), at(100)))
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		if id <= 0 {
			t.Fatalf("CreateCampaign id = %d, want > 0", id)
		}

		got, err := repo.GetCampaign(ctx, id)
		if err != nil {
			t.Fatalf("GetCampaign: %v", err)
		}
		if got.Name != "Supersale 11.11" || got.TenantID != 7 {
			t.Fatalf("GetCampaign = %+v, want name/tenant round-tripped", got)
		}
		if !got.Window.Start.Equal(at(0)) || !got.Window.End.Equal(at(100)) {
			t.Fatalf("GetCampaign window = %+v, want [0, 100)", got.Window)
		}
		if got.AbortedAt != nil {
			t.Fatalf("GetCampaign AbortedAt = %v, want nil for a freshly created campaign", got.AbortedAt)
		}
		wantServices := map[int64]int64{1: 10, 2: 20}
		if len(got.Services) != len(wantServices) {
			t.Fatalf("GetCampaign services = %+v, want %d entries", got.Services, len(wantServices))
		}
		for _, svc := range got.Services {
			if wantServices[svc.ProjectID] != svc.ExecutionID {
				t.Errorf("service project %d -> execution %d, want %d", svc.ProjectID, svc.ExecutionID, wantServices[svc.ProjectID])
			}
		}
	})

	t.Run("GetMissingReturnsNotFound", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.GetCampaign(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetCampaign(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("ListCampaignsByTenantOrderedByWindowStartAndScoped", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		second, err := repo.CreateCampaign(ctx, testCampaign(7, at(200), at(300)))
		if err != nil {
			t.Fatalf("CreateCampaign (second, tenant 7): %v", err)
		}
		first, err := repo.CreateCampaign(ctx, testCampaign(7, at(0), at(100)))
		if err != nil {
			t.Fatalf("CreateCampaign (first, tenant 7): %v", err)
		}
		if _, err := repo.CreateCampaign(ctx, testCampaign(9, at(0), at(100))); err != nil {
			t.Fatalf("CreateCampaign (tenant 9): %v", err)
		}

		got, err := repo.ListCampaignsByTenant(ctx, 7)
		if err != nil {
			t.Fatalf("ListCampaignsByTenant: %v", err)
		}
		if len(got) != 2 || got[0].ID != first || got[1].ID != second {
			t.Fatalf("ListCampaignsByTenant = %+v, want [first, second] ordered by window start", got)
		}
	})

	// The exact case both lifecycleapp's freeze check and cmd/scheduler's
	// drain sweep rely on: a campaign not yet started, one already ended,
	// and one aborted mid-window must all be excluded, even though an
	// aborted one's window still numerically contains now.
	t.Run("ListActiveCampaignsExcludesNotYetStartedEndedAndAborted", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		active, err := repo.CreateCampaign(ctx, testCampaign(7, at(0), at(100)))
		if err != nil {
			t.Fatalf("CreateCampaign (active): %v", err)
		}
		if _, err := repo.CreateCampaign(ctx, testCampaign(7, at(200), at(300))); err != nil {
			t.Fatalf("CreateCampaign (not yet started): %v", err)
		}
		if _, err := repo.CreateCampaign(ctx, testCampaign(7, at(-200), at(-100))); err != nil {
			t.Fatalf("CreateCampaign (already ended): %v", err)
		}
		aborted, err := repo.CreateCampaign(ctx, testCampaign(7, at(0), at(100)))
		if err != nil {
			t.Fatalf("CreateCampaign (to be aborted): %v", err)
		}
		if err := repo.AbortCampaign(ctx, aborted, at(50)); err != nil {
			t.Fatalf("AbortCampaign: %v", err)
		}

		got, err := repo.ListActiveCampaigns(ctx, at(50))
		if err != nil {
			t.Fatalf("ListActiveCampaigns: %v", err)
		}
		if len(got) != 1 || got[0].ID != active {
			t.Fatalf("ListActiveCampaigns(at 50) = %+v, want only the one truly-active campaign (%d)", got, active)
		}
	})

	t.Run("AbortCampaign", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id, err := repo.CreateCampaign(ctx, testCampaign(7, at(0), at(100)))
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}

		if err := repo.AbortCampaign(ctx, id, at(50)); err != nil {
			t.Fatalf("AbortCampaign: %v", err)
		}
		got, err := repo.GetCampaign(ctx, id)
		if err != nil {
			t.Fatalf("GetCampaign: %v", err)
		}
		if got.AbortedAt == nil || !got.AbortedAt.Equal(at(50)) {
			t.Fatalf("GetCampaign AbortedAt = %v, want %v", got.AbortedAt, at(50))
		}

		// Idempotent: aborting an already-aborted campaign again, even with
		// the exact same timestamp (the case a naive RowsAffected check
		// mishandles), must not error.
		if err := repo.AbortCampaign(ctx, id, at(50)); err != nil {
			t.Fatalf("AbortCampaign (repeat, same timestamp): %v", err)
		}

		if err := repo.AbortCampaign(ctx, 999, at(50)); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("AbortCampaign(missing) = %v, want ErrNotFound", err)
		}
	})
}
