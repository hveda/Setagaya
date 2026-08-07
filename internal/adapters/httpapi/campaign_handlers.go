package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/rbac"
)

type serviceResponse struct {
	ProjectID   int64 `json:"project_id"`
	ExecutionID int64 `json:"execution_id"`
}

type campaignResponse struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	TenantID    int64             `json:"tenant_id"`
	WindowStart time.Time         `json:"window_start"`
	WindowEnd   time.Time         `json:"window_end"`
	Services    []serviceResponse `json:"services"`
	// Active/AbortedAt surface campaign.Campaign.IsActive's derived state, so
	// a caller never has to reimplement the window+abort computation itself.
	Active    bool       `json:"active"`
	AbortedAt *time.Time `json:"aborted_at,omitempty"`
}

func toCampaignResponse(c campaign.Campaign, now time.Time) campaignResponse {
	services := make([]serviceResponse, len(c.Services))
	for i, svc := range c.Services {
		services[i] = serviceResponse{ProjectID: svc.ProjectID, ExecutionID: svc.ExecutionID}
	}
	return campaignResponse{
		ID: c.ID, Name: c.Name, TenantID: c.TenantID,
		WindowStart: c.Window.Start, WindowEnd: c.Window.End,
		Services: services, Active: c.IsActive(now), AbortedAt: c.AbortedAt,
	}
}

// parseCampaignServices reads a campaign's participating services from
// paired, index-matched form values (service_project_id[i] designates
// service_execution_id[i] as that project's readiness test) -- the
// form-encoded shape closest to a list of pairs without switching this one
// endpoint to a JSON body while every other mutating route stays
// form-encoded.
func parseCampaignServices(form url.Values) ([]campaign.Service, error) {
	projectIDs := form["service_project_id"]
	executionIDs := form["service_execution_id"]
	if len(projectIDs) != len(executionIDs) {
		return nil, fmt.Errorf("service_project_id and service_execution_id must have the same number of values")
	}
	out := make([]campaign.Service, len(projectIDs))
	for i := range projectIDs {
		projectID, err := strconv.ParseInt(projectIDs[i], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid service_project_id %q", projectIDs[i])
		}
		executionID, err := strconv.ParseInt(executionIDs[i], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid service_execution_id %q", executionIDs[i])
		}
		out[i] = campaign.Service{ProjectID: projectID, ExecutionID: executionID}
	}
	return out, nil
}

// createCampaign defines a PM-owned readiness event: a window and the
// services (projects, each with its designated readiness execution)
// participating in it.
func (h *handlers) createCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.campaignsConfigured(w) {
		return
	}
	tenantID, ok := pathInt(r, "tenant_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tenant id")
		return
	}
	if err := h.authorizeCampaignTenant(r.Context(), tenantID, rbac.ActionCreate); err != nil {
		respondError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	start, err := time.Parse(time.RFC3339, r.PostForm.Get("window_start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid window_start: "+err.Error())
		return
	}
	end, err := time.Parse(time.RFC3339, r.PostForm.Get("window_end"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid window_end: "+err.Error())
		return
	}
	services, err := parseCampaignServices(r.PostForm)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c := campaign.Campaign{
		Name:     r.PostForm.Get("name"),
		TenantID: tenantID,
		Window:   campaign.Window{Start: start, End: end},
		Services: services,
	}
	created, err := h.deps.Campaigns.Create(r.Context(), c)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCampaignResponse(created, time.Now()))
}

// listCampaigns lists every campaign belonging to tenantID.
func (h *handlers) listCampaigns(w http.ResponseWriter, r *http.Request) {
	if !h.campaignsConfigured(w) {
		return
	}
	tenantID, ok := pathInt(r, "tenant_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tenant id")
		return
	}
	if err := h.authorizeCampaignTenant(r.Context(), tenantID, rbac.ActionList); err != nil {
		respondError(w, err)
		return
	}
	campaigns, err := h.deps.Campaigns.List(r.Context(), tenantID)
	if err != nil {
		respondError(w, err)
		return
	}
	now := time.Now()
	out := make([]campaignResponse, len(campaigns))
	for i, c := range campaigns {
		out[i] = toCampaignResponse(c, now)
	}
	writeJSON(w, http.StatusOK, out)
}

// getCampaign returns one campaign. Authorization is derived from the
// campaign's own recorded TenantID, not any tenant_id the caller might
// supply, the same pattern authorizeSchedule uses for a schedule's
// execution id.
func (h *handlers) getCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.campaignsConfigured(w) {
		return
	}
	id, ok := pathInt(r, "campaign_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}
	c, err := h.deps.Campaigns.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	if err := h.authorizeCampaignTenant(r.Context(), c.TenantID, rbac.ActionRead); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCampaignResponse(c, time.Now()))
}

// campaignsConfigured rejects the request with 404 unless campaigns are
// wired (Deps.Campaigns != nil), mirroring tenantAdminGate's own guard for
// an optional dependency. Returns true when the handler may proceed.
func (h *handlers) campaignsConfigured(w http.ResponseWriter) bool {
	if h.deps.Campaigns == nil {
		writeError(w, http.StatusNotFound, "campaigns not configured")
		return false
	}
	return true
}

// authorizeCampaignTenant checks the caller may act on tenantID's campaigns
// with action. Under RBAC this requires a role, global or scoped to
// tenantID specifically, granting rbac.ResourceCampaign/action --
// RoleCampaignManager is the only built-in role with that permission, so an
// ordinary tenant editor/admin (who can manage the same tenant's projects)
// is still denied: a campaign freezes other teams' work, and that authority
// is deliberately not bundled with project edit rights. In no-auth mode the
// operator has full access, matching every other admin-adjacent route.
func (h *handlers) authorizeCampaignTenant(ctx context.Context, tenantID int64, action rbac.Action) error {
	if !h.rbacEnabled() {
		return nil
	}
	dec := h.deps.Auth.Authorize(accountFrom(ctx), rbac.Request{Resource: rbac.ResourceCampaign, Action: action, TenantID: &tenantID})
	if !dec.Allowed {
		return errForbidden
	}
	return nil
}
