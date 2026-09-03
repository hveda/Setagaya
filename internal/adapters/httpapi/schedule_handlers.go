package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/domain/rbac"
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
//
// tenant_id is still taken from the request (execution.TenantID is never
// populated by any current code path, so there is no server-side source to
// derive it from -- see authorizeScheduleTenant), but it is no longer
// trusted unchecked: authorizeScheduleTenant requires the caller be
// authorized for that specific tenant, the same rule createProject already
// applies to a client-declared tenant_id, so a caller cannot attribute a
// schedule's quota to a tenant they have no relationship to.
func (h *handlers) createSchedule(w http.ResponseWriter, r *http.Request) {
	executionID, ok := pathInt(r, "execution_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution id")
		return
	}
	if err := h.authorizeExecution(r, executionID, rbac.ActionCreate); err != nil {
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
	if err := h.authorizeScheduleTenant(r.Context(), tenantID); err != nil {
		respondError(w, err)
		return
	}
	sc := schedule.Schedule{
		ExecutionID: executionID,
		TenantID:    tenantID,
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
	if err := h.authorizeExecution(r, executionID, rbac.ActionRead); err != nil {
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
	return h.authorizeExecution(r, sc.ExecutionID, rbac.ActionDelete)
}

// authorizeScheduleTenant checks the caller may attribute a schedule's
// occurrences -- and the quota they reserve -- to tenantID. Under RBAC this
// requires a role, global or scoped to tenantID specifically, granting
// rbac.ResourceProject/ActionUpdate: the same permission authorizeExecution
// already requires for the execution's own project, and the same pattern
// createProject already applies to its own client-declared tenant_id.
// Without this, authorizeExecution alone only proves the caller may manage
// the execution -- not that tenantID is a tenant they have any relationship
// to. In no-auth mode authorizeExecution has already fully vetted the
// caller, so tenant_id is accepted as given, matching every other no-auth
// route.
func (h *handlers) authorizeScheduleTenant(ctx context.Context, tenantID int64) error {
	if !h.rbacEnabled() {
		return nil
	}
	dec := h.deps.Auth.Authorize(accountFrom(ctx), rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionUpdate, TenantID: &tenantID})
	if !dec.Allowed {
		return errForbidden
	}
	return nil
}
