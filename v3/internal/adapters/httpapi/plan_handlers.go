package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/app/planapp"
	"github.com/heridotlife/Setagaya/v3/internal/domain/plan"
)

type planResponse struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	ProjectID   int64             `json:"project_id"`
	CreatedTime time.Time         `json:"created_time"`
	TestFile    *planapp.FileRef  `json:"test_file"`
	Data        []planapp.FileRef `json:"data"`
}

func (h *handlers) getPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "plan_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	p, err := h.deps.Plans.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	files, err := h.deps.Plans.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, planResponse{
		ID:          p.ID,
		Name:        p.Name,
		ProjectID:   p.ProjectID,
		CreatedTime: p.CreatedTime,
		TestFile:    files.TestFile,
		Data:        files.Data,
	})
}

func (h *handlers) createPlan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	projectID, err := strconv.ParseInt(r.PostForm.Get("project_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id")
		return
	}
	if err := h.authorizeProject(r.Context(), projectID); err != nil {
		respondError(w, err)
		return
	}
	p, err := h.deps.Plans.Create(r.Context(), r.PostForm.Get("name"), projectID)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPlanResponse(p))
}

func (h *handlers) deletePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "plan_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	if err := h.authorizePlan(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Plans.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "plan deleted"})
}

func (h *handlers) listPlanFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "plan_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	files, err := h.deps.Plans.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *handlers) uploadPlanFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "plan_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	if err := h.authorizePlan(r, id); err != nil {
		respondError(w, err)
		return
	}
	file, header, err := parseUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	defer func() { _ = file.Close() }()
	if err := h.deps.Plans.UploadFile(r.Context(), id, header.Filename, file); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "uploaded"})
}

func (h *handlers) deletePlanFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "plan_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	if err := h.authorizePlan(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Plans.DeleteFile(r.Context(), id, r.URL.Query().Get("filename")); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// authorizePlan loads a plan and verifies the caller owns its project.
func (h *handlers) authorizePlan(r *http.Request, planID int64) error {
	p, err := h.deps.Plans.Get(r.Context(), planID)
	if err != nil {
		return err
	}
	return h.authorizeProject(r.Context(), p.ProjectID)
}

func toPlanResponse(p plan.Plan) planResponse {
	return planResponse{
		ID:          p.ID,
		Name:        p.Name,
		ProjectID:   p.ProjectID,
		CreatedTime: p.CreatedTime,
		Data:        []planapp.FileRef{},
	}
}
