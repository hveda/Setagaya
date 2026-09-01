package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/scenario"
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

// importScenario creates a scenario from an uploaded JMeter plan and returns
// the inspector's findings alongside it. The findings are part of the success
// response rather than a warning log: a plan that runs differently under Honryu
// than it did under Shibuya is exactly what the importing user needs told.
func (h *handlers) importScenario(w http.ResponseWriter, r *http.Request) {
	file, header, err := parseUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read upload")
		return
	}
	defer func() { _ = file.Close() }()

	projectID, err := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id")
		return
	}
	if err := h.authorizeProject(r.Context(), projectID); err != nil {
		respondError(w, err)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ".jmx")
	}

	res, err := h.deps.Scenarios.ImportJMX(r.Context(), name, projectID, header.Filename, file)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"scenario": toScenarioResponse(res.Scenario),
		"report":   res.Report,
	})
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

// setScenarioRequests uploads a portable scenario's declarative workload, as
// the raw bytes of a Taurus `scenarios:` fragment -- scenarioapp.SetRequests
// does the parsing and validation, so this handler only moves bytes.
func (h *handlers) setScenarioRequests(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	if err := h.authorizeScenario(r, id); err != nil {
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
		writeError(w, http.StatusBadRequest, "failed to read requests")
		return
	}
	if err := h.deps.Scenarios.SetRequests(r.Context(), id, raw); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "requests stored"})
}

// getScenarioRequests returns a portable scenario's stored fragment exactly
// as uploaded -- raw text/yaml, not JSON-wrapped -- so the editor can
// round-trip what it edits. 404 when nothing has ever been uploaded; 409
// for a non-portable scenario, matching SetRequests' own stance.
func (h *handlers) getScenarioRequests(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "scenario_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid scenario id")
		return
	}
	if err := h.authorizeScenario(r, id); err != nil {
		respondError(w, err)
		return
	}
	raw, err := h.deps.Scenarios.Requests(r.Context(), id)
	if err != nil {
		respondError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(raw); err != nil {
		slog.Error("httpapi: failed to write requests fragment", "error", err)
	}
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
