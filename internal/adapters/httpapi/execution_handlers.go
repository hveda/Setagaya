package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"time"

	yaml "gopkg.in/yaml.v3"

	"github.com/heridotlife/Setagaya/internal/app/executionapp"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
)

type collectionResponse struct {
	ID             int64                  `json:"id"`
	Name           string                 `json:"name"`
	ProjectID      int64                  `json:"project_id"`
	CSVSplit       bool                   `json:"csv_split"`
	CreatedTime    time.Time              `json:"created_time"`
	ExecutionPlans []loadprofile.Entry    `json:"load_profile"`
	Data           []executionapp.FileRef `json:"data"`
}

func (h *handlers) getCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	c, err := h.deps.Collections.Get(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	cfg, err := h.deps.Collections.GetConfig(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	files, err := h.deps.Collections.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, collectionResponse{
		ID:             c.ID,
		Name:           c.Name,
		ProjectID:      c.ProjectID,
		CSVSplit:       c.CSVSplit,
		CreatedTime:    c.CreatedTime,
		ExecutionPlans: cfg.Content.Tests,
		Data:           files,
	})
}

func (h *handlers) createCollection(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.deps.Collections.Create(r.Context(), r.PostForm.Get("name"), projectID)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCollectionResponse(c))
}

func (h *handlers) deleteCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := h.authorizeCollection(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Collections.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "collection deleted"})
}

func (h *handlers) listCollectionFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	files, err := h.deps.Collections.Files(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *handlers) uploadCollectionFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := h.authorizeCollection(r, id); err != nil {
		respondError(w, err)
		return
	}
	file, header, err := parseUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file upload")
		return
	}
	defer func() { _ = file.Close() }()
	if err := h.deps.Collections.UploadFile(r.Context(), id, header.Filename, file); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "uploaded"})
}

func (h *handlers) deleteCollectionFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := h.authorizeCollection(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Collections.DeleteFile(r.Context(), id, r.URL.Query().Get("filename")); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (h *handlers) uploadCollectionConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	if err := h.authorizeCollection(r, id); err != nil {
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
	if err := h.deps.Collections.StoreConfig(r.Context(), id, wrapper.Content); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "config stored"})
}

func (h *handlers) getCollectionConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	cfg, err := h.deps.Collections.GetConfig(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// authorizeCollection loads a collection and verifies the caller owns its
// project.
func (h *handlers) authorizeCollection(r *http.Request, executionID int64) error {
	c, err := h.deps.Collections.Get(r.Context(), executionID)
	if err != nil {
		return err
	}
	return h.authorizeProject(r.Context(), c.ProjectID)
}

func toCollectionResponse(c execution.Execution) collectionResponse {
	return collectionResponse{
		ID:             c.ID,
		Name:           c.Name,
		ProjectID:      c.ProjectID,
		CSVSplit:       c.CSVSplit,
		CreatedTime:    c.CreatedTime,
		ExecutionPlans: []loadprofile.Entry{},
		Data:           []executionapp.FileRef{},
	}
}
