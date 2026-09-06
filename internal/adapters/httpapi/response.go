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

// writeError writes a JSON error envelope: the message-only shape every
// existing consumer pins.
func writeError(w http.ResponseWriter, status int, message string) {
	writeErrorDetails(w, status, message, nil)
}

// writeErrorDetails writes the envelope with an optional structured
// details payload (phase 24): {"message": ..., "details": {...}}. The
// message stays the stable contract; details is strictly opt-in per error,
// and a nil details omits the key entirely so message-only bodies remain
// byte-identical to the pre-envelope encoding.
func writeErrorDetails(w http.ResponseWriter, status int, message string, details any) {
	if details == nil {
		writeJSON(w, status, map[string]string{"message": message})
		return
	}
	writeJSON(w, status, map[string]any{"message": message, "details": details})
}
