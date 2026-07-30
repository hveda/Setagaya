package httpapi_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
)

// TestOpenAPIMatchesRoutes fails when the router and the published OpenAPI
// document drift apart. The route table is the source of truth; the document
// must describe exactly the routes that exist, no more and no fewer.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	t.Parallel()

	doc := loadOpenAPI(t)

	documented := make(map[string]bool)
	for path, item := range doc.Paths {
		for key := range item {
			// A path item legally holds non-operation keys ("parameters",
			// "summary", "description", "servers", "$ref"). Only the HTTP
			// methods describe routes.
			if !isHTTPMethod(key) {
				continue
			}
			documented[strings.ToUpper(key)+" "+path] = true
		}
	}

	registered := make(map[string]bool)
	for _, r := range httpapi.Routes() {
		// The OpenAPI path syntax is {param}, same as net/http's, so patterns
		// compare directly.
		registered[r.Method+" "+r.Pattern] = true
	}

	for route := range registered {
		if !documented[route] {
			t.Errorf("route %q is registered but missing from api/openapi.yaml", route)
		}
	}
	for route := range documented {
		if !registered[route] {
			t.Errorf("route %q is documented but not registered in the router", route)
		}
	}

	if t.Failed() {
		t.Logf("registered routes:\n  %s", strings.Join(sortedKeys(registered), "\n  "))
		t.Logf("documented routes:\n  %s", strings.Join(sortedKeys(documented), "\n  "))
	}
}

// TestOpenAPITagsMatchRouteGroups keeps the document's tags trustworthy: each
// operation's tag must be the group its route declares, so the two cannot drift.
func TestOpenAPITagsMatchRouteGroups(t *testing.T) {
	t.Parallel()

	doc := loadOpenAPI(t)

	for _, r := range httpapi.Routes() {
		item, ok := doc.Paths[r.Pattern]
		if !ok {
			continue // reported by TestOpenAPIMatchesRoutes
		}
		op, ok := item[strings.ToLower(r.Method)].(map[string]any)
		if !ok {
			continue
		}
		tags, _ := op["tags"].([]any)
		if len(tags) == 0 {
			t.Errorf("%s %s: operation has no tags, want %q", r.Method, r.Pattern, r.Group)
			continue
		}
		if got, _ := tags[0].(string); got != r.Group {
			t.Errorf("%s %s: documented tag %q, route group %q", r.Method, r.Pattern, got, r.Group)
		}
	}
}

// TestRouteTableWellFormed checks the route table itself: entries are complete,
// patterns are unique, and every pattern is one net/http's ServeMux accepts --
// the last verified by building a router, which panics on a malformed or
// duplicate pattern.
func TestRouteTableWellFormed(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, r := range httpapi.Routes() {
		if r.Method == "" || r.Pattern == "" {
			t.Errorf("route with empty method or pattern: %+v", r)
		}
		if !strings.HasPrefix(r.Pattern, "/") {
			t.Errorf("route pattern %q does not start with /", r.Pattern)
		}
		if r.Group == "" {
			t.Errorf("route %s %s has no group", r.Method, r.Pattern)
		}
		key := r.Method + " " + r.Pattern
		if seen[key] {
			t.Errorf("duplicate route %q", key)
		}
		seen[key] = true
	}

	// Registers all patterns; ServeMux panics on an invalid or duplicate one.
	if h := httpapi.NewRouter(httpapi.Deps{}); h == nil {
		t.Fatal("NewRouter returned nil")
	}
}

// httpMethods are the operation keys OpenAPI defines for a path item.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

func isHTTPMethod(key string) bool { return httpMethods[strings.ToLower(key)] }

type openAPIDoc struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

func loadOpenAPI(t *testing.T) openAPIDoc {
	t.Helper()
	// test file lives in internal/adapters/httpapi
	path := filepath.Join("..", "..", "..", "api", "openapi.yaml")
	data, err := os.ReadFile(path) //nolint:gosec // fixed repo-relative path
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var doc openAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("openapi.yaml declares no paths")
	}
	return doc
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
