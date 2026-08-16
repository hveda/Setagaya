package campaignapp

import (
	"context"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// CampaignComparison is a campaign compared to a baseline: one row per
// project participating in either. HasBaseline is false when the target
// campaign has no resolvable baseline (no prior ended campaign in its
// tenant, and none given explicitly) -- in that case Services is empty
// rather than computed against an empty baseline, since "no baseline
// campaign" and "a baseline campaign with no participating services" are
// different facts and must not be conflated.
type CampaignComparison struct {
	CampaignID         int64
	HasBaseline        bool
	BaselineCampaignID int64
	Services           []campaign.ServiceComparison
}

// Compare classifies campaignID's services against a baseline: baselineID if
// positive (an explicit override), otherwise the tenant's most-recent-prior
// *ended* campaign -- the tenant's other campaign, other than campaignID
// itself, with the latest window start strictly before campaignID's own,
// among those that have already ended (aborted, or their window has closed).
// Both verdicts are computed via Verdict, so the target-QPS gate (task 103)
// applies identically on both sides of the comparison.
func (s *Service) Compare(ctx context.Context, campaignID int64, baselineID int64) (CampaignComparison, error) {
	c, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return CampaignComparison{}, err
	}

	var baseline campaign.Campaign
	hasBaseline := false
	if baselineID > 0 {
		baseline, err = s.repo.GetCampaign(ctx, baselineID)
		if err != nil {
			return CampaignComparison{}, err
		}
		hasBaseline = true
	} else {
		baseline, hasBaseline, err = s.resolveDefaultBaseline(ctx, c)
		if err != nil {
			return CampaignComparison{}, err
		}
	}

	out := CampaignComparison{CampaignID: campaignID, HasBaseline: hasBaseline}
	if !hasBaseline {
		return out, nil
	}
	out.BaselineCampaignID = baseline.ID

	currentVerdict, err := s.Verdict(ctx, campaignID)
	if err != nil {
		return CampaignComparison{}, err
	}
	baselineVerdict, err := s.Verdict(ctx, baseline.ID)
	if err != nil {
		return CampaignComparison{}, err
	}

	out.Services = campaign.Compare(toSignals(currentVerdict.Services), toSignals(baselineVerdict.Services))
	return out, nil
}

// resolveDefaultBaseline finds the tenant's most-recent-prior ended campaign,
// other than c itself: "ended" means aborted or its window has closed by now;
// "prior" means its window started strictly before c's own. Among matches,
// the one with the latest window start wins. found is false when none exist.
func (s *Service) resolveDefaultBaseline(ctx context.Context, c campaign.Campaign) (baseline campaign.Campaign, found bool, err error) {
	all, err := s.repo.ListCampaignsByTenant(ctx, c.TenantID)
	if err != nil {
		return campaign.Campaign{}, false, err
	}
	now := s.now()
	for _, other := range all {
		if other.ID == c.ID {
			continue
		}
		if !hasEnded(other, now) {
			continue
		}
		if !other.Window.Start.Before(c.Window.Start) {
			continue
		}
		if !found || other.Window.Start.After(baseline.Window.Start) {
			baseline, found = other, true
		}
	}
	return baseline, found, nil
}

// hasEnded reports whether c's readiness window has concluded as of now:
// aborted, or its window end has passed. A campaign whose window has not yet
// closed (even if not currently active because it hasn't started) is not a
// candidate baseline -- it has nothing final to compare against yet.
func hasEnded(c campaign.Campaign, now time.Time) bool {
	return c.AbortedAt != nil || !now.Before(c.Window.End)
}

// toSignals reduces each ServiceVerdict to the binary go/no-go signal
// campaign.Compare classifies on -- the same condition Verdict's own overall
// Go gate applies per service (verdict.go): has a report, it passed, and it
// is not short of its target QPS.
func toSignals(services []ServiceVerdict) []campaign.ServiceSignal {
	out := make([]campaign.ServiceSignal, len(services))
	for i, sv := range services {
		out[i] = campaign.ServiceSignal{
			ProjectID: sv.ProjectID,
			Go:        sv.HasReport && sv.Outcome == taurus.OutcomePassed && !sv.ShortOfTargetQPS,
		}
	}
	return out
}
