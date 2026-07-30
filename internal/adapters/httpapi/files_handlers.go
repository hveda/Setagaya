package httpapi

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"
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
func (h *handlers) downloadFile(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "scenario" && kind != "execution" {
		writeError(w, http.StatusBadRequest, "invalid kind")
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
