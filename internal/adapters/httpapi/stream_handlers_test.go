package httpapi_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	membus "github.com/heridotlife/Setagaya/internal/adapters/eventbus/memory"
	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/domain/engine"
)

func TestStreamCollection_DeliversSSE(t *testing.T) {
	t.Parallel()
	bus := membus.New()
	router := httpapi.NewRouter(httpapi.Deps{Events: bus, DefaultOwners: []string{"setagaya"}})
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/collections/5/stream", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Publish until the handler has subscribed and delivered an event.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				bus.Publish(5, engine.Metric{Label: "home", Latency: 12.5})
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	line := readSSEData(t, resp.Body)
	if !strings.Contains(line, `"label":"home"`) {
		t.Fatalf("SSE data = %q", line)
	}
}

func TestStreamCollection_InvalidID(t *testing.T) {
	t.Parallel()
	bus := membus.New()
	router := httpapi.NewRouter(httpapi.Deps{Events: bus, DefaultOwners: []string{"setagaya"}})
	rec := do(t, router, http.MethodGet, "/api/collections/x/stream")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid id = %d, want 400", rec.Code)
	}
}

// readSSEData reads until the first "data: " line, returning its payload, or
// fails after a deadline.
func readSSEData(t *testing.T, r interface{ Read([]byte) (int, error) }) string {
	t.Helper()
	type line struct {
		s   string
		err error
	}
	lines := make(chan line, 1)
	go func() {
		br := bufio.NewReader(r)
		for {
			s, err := br.ReadString('\n')
			if data, ok := strings.CutPrefix(s, "data: "); ok {
				lines <- line{s: strings.TrimSpace(data)}
				return
			}
			if err != nil {
				lines <- line{err: err}
				return
			}
		}
	}()
	select {
	case l := <-lines:
		if l.err != nil {
			t.Fatalf("read SSE: %v", l.err)
		}
		return l.s
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SSE data")
		return ""
	}
}
