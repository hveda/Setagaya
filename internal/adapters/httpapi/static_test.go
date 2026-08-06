package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// newAdminRouterWithStatic wires the same admin surface newAdminRouter uses
// (admin_abort_test.go), plus StaticAssets, to prove the two coexist.
func newAdminRouterWithStatic(t *testing.T) (http.Handler, *fake.Store, *fake.Scheduler) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	sched := fake.NewScheduler()
	lifecycle := lifecycleapp.NewService(store, sched, obj, lifecycleapp.StaticImage("honryu/jmeter:latest"))
	h := httpapi.NewRouter(httpapi.Deps{
		Admin:        adminapp.NewService(store, sched, lifecycle),
		StaticAssets: fakeSPA(),
	})
	return h, store, sched
}

func fakeSPA() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>spa shell</html>")},
		"assets/app.js": {Data: []byte("console.log('hi')")},
		"favicon.svg":   {Data: []byte("<svg/>")},
	}
}

func TestStaticAssets_ServesARealFile(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{StaticAssets: fakeSPA()})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('hi')" {
		t.Fatalf("GET /assets/app.js = %d %q, want 200 with the real file content", rec.Code, rec.Body.String())
	}
}

func TestStaticAssets_RootServesIndex(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{StaticAssets: fakeSPA()})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>spa shell</html>" {
		t.Fatalf("GET / = %d %q, want 200 with index.html", rec.Code, rec.Body.String())
	}
}

// A client-side route (no matching file in the build) must still serve
// index.html, not a 404 -- react-router takes it from there.
func TestStaticAssets_UnmatchedPathFallsBackToIndex(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{StaticAssets: fakeSPA()})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reports/42/detail", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>spa shell</html>" {
		t.Fatalf("GET /reports/42/detail = %d %q, want 200 with index.html (SPA fallback)", rec.Code, rec.Body.String())
	}
}

// An unmatched /api/ path must never fall back to the SPA shell -- it should
// read as an ordinary API 404, not accidentally-served HTML.
func TestStaticAssets_UnmatchedAPIPathIsJSONNotFound(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{StaticAssets: fakeSPA()})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/this-route-does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/... (unmatched) = %d, want 404", rec.Code)
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if body.Message == "" {
		t.Fatalf("body = %s, want a JSON message envelope, not the SPA shell", rec.Body.String())
	}
}

// A registered /api/ route must keep working exactly as before when static
// assets are also wired -- the catch-all "/" pattern must never shadow a
// more specific one.
func TestStaticAssets_DoesNotShadowRegisteredAPIRoutes(t *testing.T) {
	t.Parallel()
	h, _, _ := newAdminRouterWithStatic(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/nodes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/admin/nodes (registered route, static assets also wired) = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

// Without StaticAssets wired (nil, the zero value), the router behaves
// exactly as it did before this feature existed: no catch-all is
// registered, so an unmatched path is a plain 404, not index.html.
func TestStaticAssets_NilDisablesStaticServing(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET / with no StaticAssets = %d, want 404 (unchanged behaviour)", rec.Code)
	}
}
