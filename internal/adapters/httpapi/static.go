package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// newStaticHandler serves the SPA's build output (distFS, already unwrapped
// from web.Dist's "dist/" prefix by the caller -- see cmd/api's wiring): a
// real file as-is, and any unmatched non-/api/ path falls back to
// index.html so client-side routing (react-router) can take over. An
// unmatched /api/ path still gets an ordinary JSON 404 rather than the SPA
// shell -- this handler only ever runs for a request no more specific
// /api/... route already claimed, so this is the one place left that could
// otherwise mistakenly hand back HTML for what was clearly meant to be an
// API call.
func newStaticHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(distFS, path); err != nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
