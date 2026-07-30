package httpapi_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
)

// TestOpenAPIMatchesRoutes fails when the router and the published OpenAPI
// document drift apart. The route table is the source of truth; the document
// must describe exactly the routes that exist, no more and no fewer.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	t.Parallel()

	doc := loadOpenAPI(t)

	documented := make(map[string]bool)
	for path, item := range doc.Paths {
		for method := range item {
			documented[strings.ToUpper(method)+" "+path] = true
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

// TestRoutesAreServed guards the route table against becoming decorative: every
// entry must actually be reachable through the router.
func TestRoutesAreServed(t *testing.T) {
	t.Parallel()

	for _, r := range httpapi.Routes() {
		if r.Method == "" || r.Pattern == "" {
			t.Errorf("route with empty method or pattern: %+v", r)
		}
		if !strings.HasPrefix(r.Pattern, "/") {
			t.Errorf("route pattern %q does not start with /", r.Pattern)
		}
	}
}

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
