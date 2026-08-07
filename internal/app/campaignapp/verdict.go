package campaignapp

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// ServiceVerdict is one participating service's contribution to a
// campaign's rolled-up verdict, read from its designated execution's own
// latest report.
type ServiceVerdict struct {
	ProjectID   int64
	ExecutionID int64
	// HasReport is false when the designated execution has never produced
	// a report at all -- distinct from a report that exists and already
	// says "passed". A service with no report cannot contribute a go.
	HasReport bool
	Outcome   taurus.Outcome
	// FailingCriteria names the execution's configured criteria that
	// triggered against its latest report, within the evaluator's
	// supported subset (see report.Report.EvaluateCriteria). Only
	// populated when Outcome is taurus.OutcomeFailed -- an aborted or
	// errored run's "criteria" are not a meaningful pass/fail signal, and a
	// passed run has nothing to name.
	FailingCriteria []report.FailedCriterion
}

// CampaignVerdict is a campaign's rolled-up go/no-go: one entry per
// participating service, plus one overall decision.
type CampaignVerdict struct {
	CampaignID int64
	Services   []ServiceVerdict
	// Go is true only when every service has a report and that report's
	// outcome is taurus.OutcomePassed.
	Go bool
}

// Verdict computes the campaign's rolled-up verdict from each participating
// service's designated execution's latest report. Does not require the
// campaign to be active or closed -- a caller may check progress mid-window
// as easily as a final verdict once it ends.
func (s *Service) Verdict(ctx context.Context, campaignID int64) (CampaignVerdict, error) {
	c, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return CampaignVerdict{}, err
	}

	v := CampaignVerdict{CampaignID: campaignID, Go: true}
	for _, svc := range c.Services {
		sv, err := s.serviceVerdict(ctx, svc)
		if err != nil {
			return CampaignVerdict{}, err
		}
		if !sv.HasReport || sv.Outcome != taurus.OutcomePassed {
			v.Go = false
		}
		v.Services = append(v.Services, sv)
	}
	return v, nil
}

func (s *Service) serviceVerdict(ctx context.Context, svc campaign.Service) (ServiceVerdict, error) {
	sv := ServiceVerdict{ProjectID: svc.ProjectID, ExecutionID: svc.ExecutionID}

	reports, err := s.repo.ListReports(ctx, svc.ExecutionID, 1)
	if err != nil {
		return ServiceVerdict{}, err
	}
	if len(reports) == 0 {
		return sv, nil
	}
	r := reports[0]
	sv.HasReport = true
	sv.Outcome = r.Outcome

	if r.Outcome == taurus.OutcomeFailed {
		criteria, err := s.repo.CriteriaFor(ctx, svc.ExecutionID)
		if err != nil {
			return ServiceVerdict{}, err
		}
		sv.FailingCriteria = r.EvaluateCriteria(criteria)
	}
	return sv, nil
}
