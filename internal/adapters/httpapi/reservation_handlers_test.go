package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/tenantapp"
	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newReservationRouter(t *testing.T) (http.Handler, *fake.Store) {
	t.Helper()
	store := fake.NewStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Tenants:      tenantapp.NewService(store, store, store),
		Reservations: store,
	})
	return h, store
}

type reservationRow struct {
	ID          int64     `json:"id"`
	TenantID    int64     `json:"tenant_id"`
	Cluster     string    `json:"cluster"`
	EngineCount int       `json:"engine_count"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	ExecutionID int64     `json:"execution_id"`
}

func TestTenantReservations_ListsWithinRequestedWindow(t *testing.T) {
	t.Parallel()
	h, store := newReservationRouter(t)
	ctx := context.Background()
	tenantID := decodeID(t, postForm(t, h, "/api/tenants", url.Values{"name": {"acme"}, "display_name": {"Acme"}}))

	inWindow := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	outsideWindow := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "", EngineCount: 3, Start: inWindow, End: inWindow.Add(time.Hour), ExecutionID: 100,
	}); err != nil {
		t.Fatalf("CreateReservation (in window): %v", err)
	}
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "", EngineCount: 5, Start: outsideWindow, End: outsideWindow.Add(time.Hour), ExecutionID: 200,
	}); err != nil {
		t.Fatalf("CreateReservation (outside window): %v", err)
	}

	query := url.Values{
		"from": {"2026-01-01T00:00:00Z"},
		"to":   {"2026-02-01T00:00:00Z"},
	}
	rec := do(t, h, http.MethodGet, "/api/tenants/"+itoa(tenantID)+"/reservations?"+query.Encode())
	if rec.Code != http.StatusOK {
		t.Fatalf("list reservations = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []reservationRow
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].ExecutionID != 100 || got[0].EngineCount != 3 {
		t.Fatalf("reservations = %+v, want only the in-window one (execution 100)", got)
	}
}

func TestTenantReservations_DefaultWindowIsNextSevenDays(t *testing.T) {
	t.Parallel()
	h, store := newReservationRouter(t)
	ctx := context.Background()
	tenantID := decodeID(t, postForm(t, h, "/api/tenants", url.Values{"name": {"acme"}, "display_name": {"Acme"}}))

	soon := time.Now().Add(2 * time.Hour)
	farFuture := time.Now().AddDate(0, 1, 0)
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "", EngineCount: 1, Start: soon, End: soon.Add(time.Hour), ExecutionID: 1,
	}); err != nil {
		t.Fatalf("CreateReservation (soon): %v", err)
	}
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "", EngineCount: 1, Start: farFuture, End: farFuture.Add(time.Hour), ExecutionID: 2,
	}); err != nil {
		t.Fatalf("CreateReservation (far future): %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/tenants/"+itoa(tenantID)+"/reservations")
	if rec.Code != http.StatusOK {
		t.Fatalf("list reservations (default window) = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []reservationRow
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ExecutionID != 1 {
		t.Fatalf("reservations (default window) = %+v, want only the one within the next 7 days", got)
	}
}

func TestTenantReservations_ClusterScoping(t *testing.T) {
	t.Parallel()
	h, store := newReservationRouter(t)
	ctx := context.Background()
	tenantID := decodeID(t, postForm(t, h, "/api/tenants", url.Values{"name": {"acme"}, "display_name": {"Acme"}}))

	now := time.Now().Add(time.Hour)
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "default", EngineCount: 1, Start: now, End: now.Add(time.Hour), ExecutionID: 1,
	}); err != nil {
		t.Fatalf("CreateReservation (default): %v", err)
	}
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "eu-west", EngineCount: 1, Start: now, End: now.Add(time.Hour), ExecutionID: 2,
	}); err != nil {
		t.Fatalf("CreateReservation (eu-west): %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/tenants/"+itoa(tenantID)+"/reservations?cluster=eu-west")
	if rec.Code != http.StatusOK {
		t.Fatalf("list reservations (cluster scoped) = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []reservationRow
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ExecutionID != 2 {
		t.Fatalf("reservations (cluster=eu-west) = %+v, want only execution 2", got)
	}
}

func TestTenantReservations_InvalidTimeParam(t *testing.T) {
	t.Parallel()
	h, _ := newReservationRouter(t)
	tenantID := decodeID(t, postForm(t, h, "/api/tenants", url.Values{"name": {"acme"}, "display_name": {"Acme"}}))

	rec := do(t, h, http.MethodGet, "/api/tenants/"+itoa(tenantID)+"/reservations?from=not-a-date")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("list reservations (bad from) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestTenantReservations_TenantsNotConfigured(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	h := httpapi.NewRouter(httpapi.Deps{Reservations: store}) // no Tenants wired

	rec := do(t, h, http.MethodGet, "/api/tenants/1/reservations")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list reservations (tenants not configured) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}
