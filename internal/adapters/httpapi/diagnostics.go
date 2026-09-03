package httpapi

import (
	"net/http"

	"github.com/heridotlife/honryu/internal/app/scenarioapp"
)

// writeDiagnostics writes the diagnostic error envelope for the
// declarative-workload endpoints: the same message/status contract as
// writeError, plus the reasons as a list anchored to source lines. Every
// other endpoint keeps writeError's envelope untouched.
func writeDiagnostics(w http.ResponseWriter, status int, message string, diags []scenarioapp.Diagnostic) {
	if diags == nil {
		diags = []scenarioapp.Diagnostic{}
	}
	writeJSON(w, status, map[string]any{
		"message":     message,
		"diagnostics": diags,
	})
}
