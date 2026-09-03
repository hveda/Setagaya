package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/rbac"
)

// projectResponse is the JSON wire shape for a Project. Keeping it separate
// from the domain type stops persistence/domain concerns leaking into the API
// contract.
type projectResponse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Owner       string    `json:"owner"`
	SID         string    `json:"sid,omitempty"`
	TenantID    *int64    `json:"tenant_id,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
	CreatedTime time.Time `json:"created_time"`
}

func toProjectResponse(p project.Project) projectResponse {
	return projectResponse{
		ID:          p.ID,
		Name:        p.Name,
		Owner:       p.Owner,
		SID:         p.SID,
		TenantID:    p.TenantID,
		CreatedBy:   p.CreatedBy,
		UpdatedBy:   p.UpdatedBy,
		CreatedTime: p.CreatedTime,
	}
}

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.visibleProjects(r)
	if err != nil {
		respondError(w, err)
		return
	}
	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// visibleProjects returns the projects the caller may see: their owned projects
// in legacy mode; every project for a service-provider admin, or the projects of
// the caller's tenants otherwise, under RBAC.
func (h *handlers) visibleProjects(r *http.Request) ([]project.Project, error) {
	if !h.rbacEnabled() {
		return h.deps.Projects.List(r.Context(), h.deps.DefaultOwners)
	}
	acct := accountFrom(r.Context())
	if acct.HasGlobalRole(rbac.RoleServiceProviderAdmin) {
		return h.deps.Projects.ListAll(r.Context())
	}
	return h.deps.Projects.ListByTenants(r.Context(), acct.TenantIDs())
}

func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "project_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	p, err := h.deps.Projects.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProjectResponse(p))
}

func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	owner := r.PostForm.Get("owner")
	tenantID, ok := parseOptionalInt(r.PostForm.Get("tenant_id"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	if h.rbacEnabled() {
		if err := h.authorize(r.Context(), owner, tenantID, rbac.ResourceProject, rbac.ActionCreate); err != nil {
			respondError(w, err)
			return
		}
	} else if !h.owns(owner) {
		writeError(w, http.StatusForbidden, "you are not a member of "+owner)
		return
	}
	p, err := h.deps.Projects.CreateInTenant(r.Context(), r.PostForm.Get("name"), owner, r.PostForm.Get("sid"), tenantID)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProjectResponse(p))
}

// parseOptionalInt parses an optional int64 form value. An empty string yields
// (nil, true); a non-numeric value yields (nil, false).
func parseOptionalInt(s string) (*int64, bool) {
	if s == "" {
		return nil, true
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, false
	}
	return &v, true
}

func (h *handlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "project_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	if err := h.authorizeProject(r.Context(), id, rbac.ActionDelete); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Projects.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "project deleted"})
}
