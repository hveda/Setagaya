package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/domain/schedule"
)

type occurrenceResponse struct {
	ID            int64     `json:"id"`
	FireTime      time.Time `json:"fire_time"`
	Status        string    `json:"status"`
	ReservationID *int64    `json:"reservation_id,omitempty"`
}

type scheduleResponse struct {
	ID          int64                `json:"id"`
	ExecutionID int64                `json:"execution_id"`
	TenantID    int64                `json:"tenant_id"`
	Cluster     string               `json:"cluster,omitempty"`
	Kind        string               `json:"kind"`
	FireAt      *time.Time           `json:"fire_at,omitempty"`
	Recurrence  string               `json:"recurrence,omitempty"`
	Active      bool                 `json:"active"`
	Occurrences []occurrenceResponse `json:"occurrences"`
}

func toScheduleResponse(v scheduleapp.ScheduleView) scheduleResponse {
	occs := make([]occurrenceResponse, len(v.Occurrences))
	for i, o := range v.Occurrences {
		occs[i] = occurrenceResponse{ID: o.ID, FireTime: o.FireTime, Status: string(o.Status), ReservationID: o.ReservationID}
	}
	return scheduleResponse{
		ID:          v.Schedule.ID,
		ExecutionID: v.Schedule.ExecutionID,
		TenantID:    v.Schedule.TenantID,
		Cluster:     v.Schedule.Cluster,
		Kind:        string(v.Schedule.Kind),
		FireAt:      v.Schedule.FireAt,
		Recurrence:  v.Schedule.Recurrence,
		Active:      v.Schedule.Active,
		Occurrences: occs,
	}
}

// createSchedule creates a time-triggered execution -- one-shot (fire_at) or
// recurring (recurrence, a cron expression) -- and reserves every occurrence
// within the 7-day admission horizon that fits the tenant's quota.
func (h *handlers) createSchedule(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, executionID); err != nil {
		respondError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	tenantID, err := strconv.ParseInt(r.PostForm.Get("tenant_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	sc := schedule.Schedule{
		ExecutionID: executionID,
		TenantID:    tenantID,
		Cluster:     r.PostForm.Get("cluster"),
		Kind:        schedule.Kind(r.PostForm.Get("kind")),
		Recurrence:  r.PostForm.Get("recurrence"),
		Active:      true,
	}
	if raw := r.PostForm.Get("fire_at"); raw != "" {
		fireAt, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid fire_at: "+parseErr.Error())
			return
		}
		sc.FireAt = &fireAt
	}
	view, err := h.deps.Schedules.Create(r.Context(), sc)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toScheduleResponse(view))
}

// listSchedules lists an execution's schedules, each with its occurrences and
// their statuses -- reserved, rejected, fired, or completed.
func (h *handlers) listSchedules(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, executionID); err != nil {
		respondError(w, err)
		return
	}
	views, err := h.deps.Schedules.List(r.Context(), executionID)
	if err != nil {
		respondError(w, err)
		return
	}
	out := make([]scheduleResponse, len(views))
	for i, v := range views {
		out[i] = toScheduleResponse(v)
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteSchedule removes a schedule and releases every occurrence that still
// holds a reservation.
func (h *handlers) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "schedule_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}
	if err := h.authorizeSchedule(r, id); err != nil {
		respondError(w, err)
		return
	}
	if err := h.deps.Schedules.Delete(r.Context(), id); err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "schedule deleted"})
}

// authorizeSchedule loads a schedule and verifies the caller owns its
// execution's project -- derived from the schedule's own recorded
// ExecutionID, not any execution_id the request URL happens to carry, the
// same pattern authorizeScenario/authorizeExecution use for their own ids.
func (h *handlers) authorizeSchedule(r *http.Request, scheduleID int64) error {
	sc, err := h.deps.Schedules.Get(r.Context(), scheduleID)
	if err != nil {
		return err
	}
	return h.authorizeExecution(r, sc.ExecutionID)
}
