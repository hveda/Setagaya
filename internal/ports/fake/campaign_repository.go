package fake

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/ports"
)

// CreateCampaign persists c and returns its assigned ID. c.Services is
// copied so a later mutation of the caller's slice cannot alias stored
// state.
func (s *Store) CreateCampaign(_ context.Context, c campaign.Campaign) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.campaignSeq++
	c.ID = s.campaignSeq
	c.Services = append([]campaign.Service(nil), c.Services...)
	s.campaigns[c.ID] = c
	return c.ID, nil
}

// GetCampaign returns the campaign with id, or ports.ErrNotFound.
func (s *Store) GetCampaign(_ context.Context, id int64) (campaign.Campaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.campaigns[id]
	if !ok {
		return campaign.Campaign{}, ports.ErrNotFound
	}
	c.Services = append([]campaign.Service(nil), c.Services...)
	return c, nil
}

// ListCampaignsByTenant returns every campaign belonging to tenantID,
// ordered by window start.
func (s *Store) ListCampaignsByTenant(_ context.Context, tenantID int64) ([]campaign.Campaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []campaign.Campaign{}
	for _, c := range s.campaigns {
		if c.TenantID == tenantID {
			c.Services = append([]campaign.Service(nil), c.Services...)
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Window.Start.Before(out[j].Window.Start) })
	return out, nil
}

// ListActiveCampaigns returns every campaign whose window contains now and
// which has not been aborted.
func (s *Store) ListActiveCampaigns(_ context.Context, now time.Time) ([]campaign.Campaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []campaign.Campaign{}
	for _, c := range s.campaigns {
		if c.IsActive(now) {
			c.Services = append([]campaign.Service(nil), c.Services...)
			out = append(out, c)
		}
	}
	return out, nil
}

// AbortCampaign records that the campaign with id was aborted at t, or
// ports.ErrNotFound.
func (s *Store) AbortCampaign(_ context.Context, id int64, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.campaigns[id]
	if !ok {
		return ports.ErrNotFound
	}
	c.AbortedAt = &t
	s.campaigns[id] = c
	return nil
}

// UpdateCampaign replaces the stored definition of the campaign with
// c.ID -- name, window, participating services -- or returns
// ports.ErrNotFound. The campaign's identity (id, tenant) and AbortedAt
// are preserved from the stored row, not taken from c. c.Services is
// copied so a later mutation of the caller's slice cannot alias stored
// state.
func (s *Store) UpdateCampaign(_ context.Context, c campaign.Campaign) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.campaigns[c.ID]
	if !ok {
		return ports.ErrNotFound
	}
	existing.Name = c.Name
	existing.Window = c.Window
	existing.Services = append([]campaign.Service(nil), c.Services...)
	s.campaigns[c.ID] = existing
	return nil
}
