package nexus_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	nexusadapter "github.com/heridotlife/honryu/internal/adapters/storage/nexus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/objectstoretest"
)

const testRepo = "honryu-raw"

// fakeNexus is an in-memory stand-in for a Nexus raw repository.
type fakeNexus struct {
	mu       sync.Mutex
	objects  map[string][]byte
	wantUser string
	wantPass string
	authSeen bool
}

func newFakeNexus() *fakeNexus {
	return &fakeNexus{objects: map[string][]byte{}}
}

func (f *fakeNexus) handler() http.Handler {
	prefix := "/repository/" + testRepo + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := strings.CutPrefix(r.URL.Path, prefix)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.wantUser != "" {
			user, pass, _ := r.BasicAuth()
			if user != f.wantUser || pass != f.wantPass {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			f.mu.Lock()
			f.authSeen = true
			f.mu.Unlock()
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			f.objects[key] = body
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			data, exists := f.objects[key]
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(data)
		case http.MethodDelete:
			if _, exists := f.objects[key]; !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(f.objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func newServer(t *testing.T, f *fakeNexus) string {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestNexus_Contract(t *testing.T) {
	t.Parallel()
	objectstoretest.RunObjectStoreContract(t, func(t *testing.T) ports.ObjectStore {
		url := newServer(t, newFakeNexus())
		return nexusadapter.New(url, testRepo)
	})
}

func TestNexus_SendsBasicAuth(t *testing.T) {
	t.Parallel()
	f := newFakeNexus()
	f.wantUser, f.wantPass = "admin", "s3cret"
	url := newServer(t, f)
	store := nexusadapter.New(url, testRepo, nexusadapter.WithBasicAuth("admin", "s3cret"))
	ctx := context.Background()

	if err := store.Upload(ctx, "scenario/1/a.jmx", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !f.authSeen {
		t.Fatal("server never saw credentials")
	}

	// Wrong credentials surface as an error.
	bad := nexusadapter.New(url, testRepo, nexusadapter.WithBasicAuth("admin", "wrong"))
	if err := bad.Upload(ctx, "scenario/1/a.jmx", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("Upload with bad creds: want error, got nil")
	}
}

func TestNexus_URL(t *testing.T) {
	t.Parallel()
	store := nexusadapter.New("https://nexus.example.com/", testRepo)
	want := "https://nexus.example.com/repository/honryu-raw/scenario/7/test.jmx"
	if got := store.URL("scenario/7/test.jmx"); got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestNexus_ServerErrorsSurface(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	store := nexusadapter.New(srv.URL, testRepo)
	ctx := context.Background()

	if err := store.Upload(ctx, "k", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("Upload on 500: want error")
	}
	if _, err := store.Download(ctx, "k"); err == nil {
		t.Fatal("Download on 500: want error")
	}
	if err := store.Delete(ctx, "k"); err == nil {
		t.Fatal("Delete on 500: want error")
	}
}

func TestNexus_RequestErrorsOnBadURL(t *testing.T) {
	t.Parallel()
	store := nexusadapter.New("http://127.0.0.1:0", testRepo)
	ctx := context.Background()

	if err := store.Upload(ctx, "k", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("Upload to dead addr: want error")
	}
	if _, err := store.Download(ctx, "k"); err == nil {
		t.Fatal("Download to dead addr: want error")
	}
	if err := store.Delete(ctx, "k"); err == nil {
		t.Fatal("Delete to dead addr: want error")
	}
}

// countingTransport records requests seen by an injected client.
type countingTransport struct {
	inner http.RoundTripper
	seen  int
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.seen++
	return c.inner.RoundTrip(r)
}

func TestNexus_WithClientUsesTheInjectedClient(t *testing.T) {
	t.Parallel()
	url := newServer(t, newFakeNexus())
	rt := &countingTransport{inner: http.DefaultTransport}
	store := nexusadapter.New(url, testRepo, nexusadapter.WithClient(&http.Client{Transport: rt}))
	ctx := context.Background()

	if err := store.Upload(ctx, "k", bytes.NewReader([]byte("v"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	got, err := store.Download(ctx, "k")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Download = %q, want %q", got, "v")
	}
	if rt.seen != 2 {
		t.Fatalf("injected transport saw %d requests, want 2", rt.seen)
	}
}

// WithClient(nil) must not clobber the default client -- optionality is
// guarded, and the default client still works end to end.
func TestNexus_WithClientNilKeepsTheDefaultClient(t *testing.T) {
	t.Parallel()
	url := newServer(t, newFakeNexus())
	store := nexusadapter.New(url, testRepo, nexusadapter.WithClient(nil))
	ctx := context.Background()

	if err := store.Upload(ctx, "k", bytes.NewReader([]byte("v"))); err != nil {
		t.Fatalf("Upload with nil client override: %v", err)
	}
	got, err := store.Download(ctx, "k")
	if err != nil {
		t.Fatalf("Download with nil client override: %v", err)
	}
	if string(got) != "v" {
		t.Fatalf("Download = %q, want %q", got, "v")
	}
}
