package campaignapp

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
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
	// OtherLoad names every other execution active in the campaign's tenant
	// during its window -- the minimum mitigation for the residual risk
	// that a non-participating execution could distort the campaign's
	// readiness numbers by contending for shared infrastructure Honryu
	// cannot see or scope around. Excludes the campaign's own designated
	// executions.
	OtherLoad []OtherLoad
}

// OtherLoad is one reservation or completed launch, other than the
// campaign's own designated executions, that overlapped its window.
type OtherLoad struct {
	ExecutionID int64
	Start       time.Time
	End         time.Time
	EngineCount int
}

// Verdict computes the campaign's rolled-up verdict from each participating
// service's designated execution's latest report. Does not require the
// campaign to be active or closed -- a caller may check progress mid-window
// as easily as a final verdict once it ends.
//
// A designated execution of Kind CalibrateEngine is skipped entirely: it
// measures the rig's own capacity, not the target's readiness, so it never
// belongs in the pass/fail rollup a campaign's go/no-go rests on.
func (s *Service) Verdict(ctx context.Context, campaignID int64) (CampaignVerdict, error) {
	c, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return CampaignVerdict{}, err
	}

	v := CampaignVerdict{CampaignID: campaignID, Go: true}
	for _, svc := range c.Services {
		exe, err := s.repo.GetExecution(ctx, svc.ExecutionID)
		if err != nil {
			return CampaignVerdict{}, err
		}
		if exe.Kind == execution.KindCalibrateEngine {
			continue
		}
		sv, err := s.serviceVerdict(ctx, svc)
		if err != nil {
			return CampaignVerdict{}, err
		}
		if !sv.HasReport || sv.Outcome != taurus.OutcomePassed {
			v.Go = false
		}
		v.Services = append(v.Services, sv)
	}

	otherLoad, err := s.otherLoad(ctx, c)
	if err != nil {
		return CampaignVerdict{}, err
	}
	v.OtherLoad = otherLoad
	return v, nil
}

// otherLoad reports every reservation or completed launch that overlapped
// c's window, other than c's own designated executions. Reservations are
// already scoped to c.TenantID by ReservationsInWindow itself; launch
// history has no such scoping (it is a global log), so records are matched
// back to an execution and dropped unless that execution belongs to
// c.TenantID -- without this, a viewer authorized only via one of c's
// participating projects would see execution ids from unrelated tenants.
func (s *Service) otherLoad(ctx context.Context, c campaign.Campaign) ([]OtherLoad, error) {
	designated := make(map[int64]bool, len(c.Services))
	for _, svc := range c.Services {
		designated[svc.ExecutionID] = true
	}

	byExecution := map[int64]OtherLoad{}
	merge := func(ol OtherLoad) {
		existing, ok := byExecution[ol.ExecutionID]
		if !ok {
			byExecution[ol.ExecutionID] = ol
			return
		}
		if ol.Start.Before(existing.Start) {
			existing.Start = ol.Start
		}
		if ol.End.After(existing.End) {
			existing.End = ol.End
		}
		if ol.EngineCount > existing.EngineCount {
			existing.EngineCount = ol.EngineCount
		}
		byExecution[ol.ExecutionID] = existing
	}

	reservations, err := s.repo.ReservationsInWindow(ctx, c.TenantID, "", c.Window.Start, c.Window.End)
	if err != nil {
		return nil, err
	}
	for _, r := range reservations {
		if designated[r.ExecutionID] {
			continue
		}
		merge(OtherLoad{ExecutionID: r.ExecutionID, Start: r.Start, End: r.End, EngineCount: r.EngineCount})
	}

	launches, err := s.repo.LaunchHistory(ctx, c.Window.Start, c.Window.End)
	if err != nil {
		return nil, err
	}
	for _, l := range launches {
		if designated[l.ExecutionID] || l.EndTime == nil {
			continue
		}
		exe, err := s.repo.GetExecution(ctx, l.ExecutionID)
		if err != nil {
			continue // gone
		}
		// execution.TenantID is never populated by any current
		// execution-creation path -- its own project's TenantID is the
		// only reliable source for which tenant this launch belongs to.
		proj, err := s.repo.GetProject(ctx, exe.ProjectID)
		if err != nil || proj.TenantID == nil || *proj.TenantID != c.TenantID {
			continue // gone, or not this campaign's tenant
		}
		merge(OtherLoad{ExecutionID: l.ExecutionID, Start: l.StartedTime, End: *l.EndTime, EngineCount: l.Engines})
	}

	out := make([]OtherLoad, 0, len(byExecution))
	for _, ol := range byExecution {
		out = append(out, ol)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExecutionID < out[j].ExecutionID })
	return out, nil
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
