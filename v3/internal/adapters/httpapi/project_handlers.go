package httpapi

import (
	"net/http"
	"time"

	"github.com/hveda/Setagaya/v3/internal/domain/project"
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
	projects, err := h.deps.Projects.List(r.Context(), h.deps.DefaultOwners)
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
	if !h.owns(owner) {
		writeError(w, http.StatusForbidden, "you are not a member of "+owner)
		return
	}
	p, err := h.deps.Projects.Create(r.Context(), r.PostForm.Get("name"), owner, r.PostForm.Get("sid"))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProjectResponse(p))
}

func (h *handlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "project_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	if err := h.authorizeProject(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Projects.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "project deleted"})
}
