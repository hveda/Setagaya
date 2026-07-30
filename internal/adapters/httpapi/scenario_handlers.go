package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/heridotlife/Setagaya/internal/app/scenarioapp"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
)

type planResponse struct {
	ID          int64                 `json:"id"`
	Name        string                `json:"name"`
	ProjectID   int64                 `json:"project_id"`
	CreatedTime time.Time             `json:"created_time"`
	TestFile    *scenarioapp.FileRef  `json:"test_file"`
	Data        []scenarioapp.FileRef `json:"data"`
}

func (h *handlers) getScenario(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	p, err := h.deps.Scenarios.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	files, err := h.deps.Scenarios.Files(r.Context(), id)
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

func (h *handlers) createScenario(w http.ResponseWriter, r *http.Request) {
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
	p, err := h.deps.Scenarios.Create(r.Context(), r.PostForm.Get("name"), projectID)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toScenarioResponse(p))
}

func (h *handlers) deleteScenario(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	if err := h.authorizeScenario(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Scenarios.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "scenario deleted"})
}

func (h *handlers) listScenarioFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	files, err := h.deps.Scenarios.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *handlers) uploadScenarioFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	if err := h.authorizeScenario(r, id); err != nil {
		respondError(w, err)
		return
	}
	file, header, err := parseUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	defer func() { _ = file.Close() }()
	if err := h.deps.Scenarios.UploadFile(r.Context(), id, header.Filename, file); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "uploaded"})
}

func (h *handlers) deleteScenarioFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	if err := h.authorizeScenario(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Scenarios.DeleteFile(r.Context(), id, r.URL.Query().Get("filename")); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// authorizeScenario loads a scenario and verifies the caller owns its project.
func (h *handlers) authorizeScenario(r *http.Request, scenarioID int64) error {
	p, err := h.deps.Scenarios.Get(r.Context(), scenarioID)
	if err != nil {
		return err
	}
	return h.authorizeProject(r.Context(), p.ProjectID)
}

func toScenarioResponse(p scenario.Scenario) planResponse {
	return planResponse{
		ID:          p.ID,
		Name:        p.Name,
		ProjectID:   p.ProjectID,
		CreatedTime: p.CreatedTime,
		Data:        []scenarioapp.FileRef{},
	}
}
