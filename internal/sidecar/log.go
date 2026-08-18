package sidecar

import (
	"fmt"
	"log/slog"
)

// logf reports a problem the sidecar recovered from. Collection continues, so
// these are warnings rather than failures: losing the rest of a run because one
// line was malformed or one push failed would be the worse outcome.
func logf(format string, args ...any) {
	slog.Warn("sidecar: " + fmt.Sprintf(format, args...))
}
