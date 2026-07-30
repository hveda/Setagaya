package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"time"

	yaml "gopkg.in/yaml.v3"

	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
)

type executionResponse struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	ProjectID   int64                  `json:"project_id"`
	CSVSplit    bool                   `json:"csv_split"`
	CreatedTime time.Time              `json:"created_time"`
	LoadProfile []loadprofile.Entry    `json:"load_profile"`
	Data        []executionapp.FileRef `json:"data"`
}

func (h *handlers) getExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	c, err := h.deps.Executions.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	cfg, err := h.deps.Executions.GetConfig(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	files, err := h.deps.Executions.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionResponse{
		ID:          c.ID,
		Name:        c.Name,
		ProjectID:   c.ProjectID,
		CSVSplit:    c.CSVSplit,
		CreatedTime: c.CreatedTime,
		LoadProfile: cfg.Content.Tests,
		Data:        files,
	})
}

func (h *handlers) createExecution(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.deps.Executions.Create(r.Context(), r.PostForm.Get("name"), projectID)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toExecutionResponse(c))
}

func (h *handlers) deleteExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Executions.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "execution deleted"})
}

func (h *handlers) listExecutionFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	files, err := h.deps.Executions.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *handlers) uploadExecutionFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, id); err != nil {
		respondError(w, err)
		return
	}
	file, header, err := parseUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	defer func() { _ = file.Close() }()
	if err := h.deps.Executions.UploadFile(r.Context(), id, header.Filename, file); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "uploaded"})
}

func (h *handlers) deleteExecutionFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Executions.DeleteFile(r.Context(), id, r.URL.Query().Get("filename")); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (h *handlers) uploadExecutionConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, id); err != nil {
		respondError(w, err)
		return
	}
	file, _, err := parseUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read config")
		return
	}
	var wrapper loadprofile.Wrapper
	if err := yaml.Unmarshal(raw, &wrapper); err != nil {
		writeError(w, http.StatusBadRequest, "invalid YAML: "+err.Error())
		return
	}
	if err := h.deps.Executions.StoreConfig(r.Context(), id, wrapper.Content); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "config stored"})
}

func (h *handlers) getExecutionConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	cfg, err := h.deps.Executions.GetConfig(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// authorizeExecution loads a execution and verifies the caller owns its
// project.
func (h *handlers) authorizeExecution(r *http.Request, executionID int64) error {
	c, err := h.deps.Executions.Get(r.Context(), executionID)
	if err != nil {
		return err
	}
	return h.authorizeProject(r.Context(), c.ProjectID)
}

func toExecutionResponse(c execution.Execution) executionResponse {
	return executionResponse{
		ID:          c.ID,
		Name:        c.Name,
		ProjectID:   c.ProjectID,
		CSVSplit:    c.CSVSplit,
		CreatedTime: c.CreatedTime,
		LoadProfile: []loadprofile.Entry{},
		Data:        []executionapp.FileRef{},
	}
}
