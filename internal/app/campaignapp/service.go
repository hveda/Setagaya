// Package campaignapp is the campaign use-case: create a PM-owned readiness
// event, list and retrieve campaigns, and abort one. It performs no I/O of
// its own beyond its Repo port.
package campaignapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/ports"
)

// ErrServiceExecutionMismatch means a campaign named an execution that does
// not actually belong to the project it was registered under.
var ErrServiceExecutionMismatch = errors.New("campaignapp: designated execution does not belong to its stated project")

// Repo is the persistence campaignapp needs: the campaign ledger, plus
// enough of an execution to verify a service's designated execution
// actually belongs to the project it's registered under.
type Repo interface {
	ports.CampaignRepository
	GetExecution(ctx context.Context, id int64) (execution.Execution, error)
}

// Service implements the campaign use-cases.
type Service struct {
	repo Repo
	now  func() time.Time
}

// NewService wires the campaign service.
func NewService(repo Repo) *Service {
	return &Service{repo: repo, now: time.Now}
}

// WithNow overrides the clock Abort timestamps with. Returns the receiver
// for chaining.
func (s *Service) WithNow(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// Create validates c and persists it. Each service's designated execution
// must actually belong to the project it's registered under -- without this
// check, a typo'd execution id would let one project's execution silently
// decide a completely different service's verdict.
func (s *Service) Create(ctx context.Context, c campaign.Campaign) (campaign.Campaign, error) {
	if err := c.Validate(); err != nil {
		return campaign.Campaign{}, err
	}
	for _, svc := range c.Services {
		exe, err := s.repo.GetExecution(ctx, svc.ExecutionID)
		if err != nil {
			return campaign.Campaign{}, err
		}
		if exe.ProjectID != svc.ProjectID {
			return campaign.Campaign{}, fmt.Errorf("%w: execution %d belongs to project %d, not %d",
				ErrServiceExecutionMismatch, svc.ExecutionID, exe.ProjectID, svc.ProjectID)
		}
	}
	id, err := s.repo.CreateCampaign(ctx, c)
	if err != nil {
		return campaign.Campaign{}, err
	}
	c.ID = id
	return c, nil
}

// Get returns the campaign with id, or ports.ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (campaign.Campaign, error) {
	return s.repo.GetCampaign(ctx, id)
}

// List returns every campaign belonging to tenantID.
func (s *Service) List(ctx context.Context, tenantID int64) ([]campaign.Campaign, error) {
	return s.repo.ListCampaignsByTenant(ctx, tenantID)
}

// Abort marks the campaign with id aborted now. Tearing down its in-scope
// executions' engines is the caller's responsibility (adminapp) -- this
// only closes the campaign itself, which is what lifts freeze immediately
// (Campaign.IsActive is derived from AbortedAt, not a separate flag).
func (s *Service) Abort(ctx context.Context, id int64) error {
	return s.repo.AbortCampaign(ctx, id, s.now())
}
