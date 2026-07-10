package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeJSON serializes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpapi: failed to encode response", "error", err)
	}
}

// writeError writes a JSON error envelope.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
