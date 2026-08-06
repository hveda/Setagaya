package httpapi

import (
	"net/http"
	"time"

	"github.com/heridotlife/honryu/internal/domain/reservation"
)

// defaultReservationWindow is used when from/to query params are omitted.
// The reservation calendar looks forward by default (matching the 7-day
// scheduling horizon), unlike usage history's backward-looking default.
const defaultReservationWindow = 7 * 24 * time.Hour

// reservationWindow parses the from/to RFC3339 query params, defaulting to
// [now, now+7d) when absent.
func reservationWindow(r *http.Request) (from, to time.Time, err error) {
	from = time.Now()
	to = from.Add(defaultReservationWindow)
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

type reservationResponse struct {
	ID          int64     `json:"id"`
	TenantID    int64     `json:"tenant_id"`
	Cluster     string    `json:"cluster,omitempty"`
	EngineCount int       `json:"engine_count"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	ExecutionID int64     `json:"execution_id"`
}

func toReservationResponse(r reservation.Reservation) reservationResponse {
	return reservationResponse{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Cluster:     r.Cluster,
		EngineCount: r.EngineCount,
		Start:       r.Start,
		End:         r.End,
		ExecutionID: r.ExecutionID,
	}
}

// tenantReservations lists a tenant's reservations for cluster within
// [from, to) -- the reservation calendar's data source. cluster defaults to
// "" (the implicit default cluster, until Phase 8's registry exists) when
// the query omits it.
func (h *handlers) tenantReservations(w http.ResponseWriter, r *http.Request) {
	if !h.tenantAdminGate(w, r) {
		return
	}
	tenantID, ok := pathInt(r, "tenant_id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tenant id")
		return
	}
	from, to, err := reservationWindow(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cluster := r.URL.Query().Get("cluster")

	reservations, err := h.deps.Reservations.ReservationsInWindow(r.Context(), tenantID, cluster, from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	out := make([]reservationResponse, len(reservations))
	for i, res := range reservations {
		out[i] = toReservationResponse(res)
	}
	writeJSON(w, http.StatusOK, out)
}
