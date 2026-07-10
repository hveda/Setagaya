package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("project_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	p, err := h.deps.Projects.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toProjectResponse(p))
}
