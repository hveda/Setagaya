package httpapi

import (
	"net/http"
	"time"
)

// defaultUsageWindow is used when from/to query params are omitted.
const defaultUsageWindow = 30 * 24 * time.Hour

// usageHistory returns finished launch records over [from, to].
func (h *handlers) usageHistory(w http.ResponseWriter, r *http.Request) {
	from, to, err := usageWindow(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	history, err := h.deps.Usage.History(r.Context(), from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

// usageSummary returns the VUH rollup over [from, to].
func (h *handlers) usageSummary(w http.ResponseWriter, r *http.Request) {
	from, to, err := usageWindow(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := h.deps.Usage.Summary(r.Context(), from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// usageWindow parses the from/to RFC3339 query params, defaulting to the last
// 30 days when absent.
func usageWindow(r *http.Request) (from, to time.Time, err error) {
	to = time.Now()
	from = to.Add(-defaultUsageWindow)
	if v := r.URL.Query().Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			return time.Time{}, time.Time{}, errBadTime("from")
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			return time.Time{}, time.Time{}, errBadTime("to")
		}
	}
	return from, to, nil
}

type badTimeError struct{ param string }

func (e badTimeError) Error() string { return "invalid " + e.param + " time; use RFC3339" }

func errBadTime(param string) error { return badTimeError{param: param} }
