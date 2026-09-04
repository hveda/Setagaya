package httpapi

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/heridotlife/honryu/internal/domain/rbac"
)

// maxUploadBytes caps multipart uploads (JMX/CSV test artifacts).
const maxUploadBytes = 100 << 20 // 100 MiB

// parseUpload extracts the single uploaded file (form field "file"). The total
// request body is capped at maxUploadBytes so an oversized upload is rejected
// before it can exhaust memory or disk (gosec G120).
func parseUpload(r *http.Request) (multipart.File, *multipart.FileHeader, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil { // #nosec G120 -- body bounded by MaxBytesReader above
		return nil, nil, err
	}
	return r.FormFile("file")
}

// downloadFile serves an artifact from the object store by kind/id/name.
// Authorization dispatches on the already-validated kind to the owning
// row's read check (spec Approach C "route-specific resolutions"):
// scenario files follow the scenario's tenant, execution files the
// execution's. The authorize helpers load the row first, so a missing
// id is a 404 and an unreadable one a 403 -- both before any object
// bytes are read.
func (h *handlers) downloadFile(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "scenario" && kind != "execution" {
		writeError(w, http.StatusBadRequest, "invalid kind")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var err error
	if kind == "scenario" {
		err = h.authorizeScenario(r, id, rbac.ActionRead)
	} else {
		err = h.authorizeExecution(r, id, rbac.ActionRead)
	}
	if err != nil {
		respondError(w, err)
		return
	}
	key := fmt.Sprintf("%s/%s/%s", kind, r.PathValue("id"), r.PathValue("name"))
	data, err := h.deps.Store.Download(r.Context(), key)
	if err != nil {
		respondError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+r.PathValue("name")+"\"")
	http.ServeContent(w, r, r.PathValue("name"), time.Time{}, bytes.NewReader(data))
}
