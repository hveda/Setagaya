package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/tenantapp"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// putForm sends a form-encoded body with an arbitrary method -- postForm
// (router_phase1_test.go) is POST-only, and PUT is what a quota update uses.
func putForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newTenantRouter(t *testing.T) http.Handler {
	t.Helper()
	store := fake.NewStore()
	return httpapi.NewRouter(httpapi.Deps{
		Tenants: tenantapp.NewService(store, store, store),
	})
}

// An unconfigured ceiling reads as 0 through the HTTP layer too, and setting
// one round-trips.
func TestTenantQuota_GetDefaultsToZeroAndSetRoundTrips(t *testing.T) {
	t.Parallel()
	h := newTenantRouter(t)

	tenantID := decodeID(t, postForm(t, h, "/api/tenants", url.Values{"name": {"acme"}, "display_name": {"Acme"}}))

	rec := do(t, h, http.MethodGet, "/api/tenants/"+itoa(tenantID)+"/quota")
	if rec.Code != http.StatusOK {
		t.Fatalf("get quota = %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ceiling":0`) {
		t.Fatalf("get quota before configured = %s, want ceiling 0", rec.Body.String())
	}

	rec = putForm(t, h, "/api/tenants/"+itoa(tenantID)+"/quota", url.Values{"ceiling": {"5"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("set quota = %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(t, h, http.MethodGet, "/api/tenants/"+itoa(tenantID)+"/quota")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ceiling":5`) {
		t.Fatalf("get quota after set = %d %s, want ceiling 5", rec.Code, rec.Body.String())
	}
}

func TestTenantQuota_RejectsNegativeCeiling(t *testing.T) {
	t.Parallel()
	h := newTenantRouter(t)
	tenantID := decodeID(t, postForm(t, h, "/api/tenants", url.Values{"name": {"acme"}, "display_name": {"Acme"}}))

	rec := putForm(t, h, "/api/tenants/"+itoa(tenantID)+"/quota", url.Values{"ceiling": {"-1"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("set negative quota = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}
