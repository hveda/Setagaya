package k6_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	k6adapter "github.com/heridotlife/Setagaya/v3/internal/adapters/executor/k6"
	"github.com/heridotlife/Setagaya/v3/internal/domain/engine"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
	"github.com/heridotlife/Setagaya/v3/internal/ports/executortest"
)

// fakeAgent stands in for the k6 sidecar agent.
type fakeAgent struct {
	mu       sync.Mutex
	running  bool
	stream   string // SSE body to serve on /stream
	startErr int    // if non-zero, /start returns this status
}

func (a *fakeAgent) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.startErr != 0 {
			w.WriteHeader(a.startErr)
			return
		}
		if a.running {
			w.WriteHeader(http.StatusConflict)
			return
		}
		a.running = true
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.running = false
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /progress", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.running {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("GET /stream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		a.mu.Lock()
		body := a.stream
		a.mu.Unlock()
		_, _ = fmt.Fprint(w, body)
	})
	return mux
}

func newAgent(t *testing.T, a *fakeAgent) string {
	t.Helper()
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestK6Executor_Contract(t *testing.T) {
	t.Parallel()
	executortest.RunExecutorContract(t, func(t *testing.T) (ports.Executor, string) {
		url := newAgent(t, &fakeAgent{})
		return k6adapter.New(nil), url
	})
}

func TestK6_KindIsK6(t *testing.T) {
	t.Parallel()
	if got := k6adapter.New(nil).Kind(); got != "k6" {
		t.Fatalf("Kind = %q, want k6", got)
	}
}

func TestK6_SubscribeParsesJSONSamples(t *testing.T) {
	t.Parallel()
	// A malformed line is skipped; a non-latency sample (vus) is skipped; two
	// http_req_duration samples are parsed.
	body := "" +
		`data: {"metric":"http_req_duration","value":12.5,"vus":8,"name":"home","status":"200"}` + "\n\n" +
		`data: {not json}` + "\n\n" +
		`data: {"metric":"vus","value":8}` + "\n\n" +
		`data: {"metric":"http_req_duration","value":30,"vus":10,"name":"login","status":"500"}` + "\n\n"
	url := newAgent(t, &fakeAgent{stream: body})
	e := k6adapter.New(nil)

	ch, err := e.Subscribe(context.Background(), url)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	var got []engine.Metric
	for m := range ch {
		got = append(got, m)
	}
	if len(got) != 2 {
		t.Fatalf("metrics = %d, want 2: %+v", len(got), got)
	}
	if got[0].Label != "home" || got[0].Status != "200" || got[0].Latency != 12.5 || got[0].Threads != 8 {
		t.Fatalf("metric[0] = %+v", got[0])
	}
	if got[1].Label != "login" || got[1].Latency != 30 || got[1].Threads != 10 {
		t.Fatalf("metric[1] = %+v", got[1])
	}
}

func TestK6_TriggerMissingScript(t *testing.T) {
	t.Parallel()
	url := newAgent(t, &fakeAgent{startErr: http.StatusNotFound})
	if err := k6adapter.New(nil).Trigger(context.Background(), url, engine.Config{}); err == nil {
		t.Fatal("Trigger with 404: want error, got nil")
	}
}

func TestK6_TriggerServerError(t *testing.T) {
	t.Parallel()
	url := newAgent(t, &fakeAgent{startErr: http.StatusInternalServerError})
	if err := k6adapter.New(nil).Trigger(context.Background(), url, engine.Config{}); err == nil {
		t.Fatal("Trigger with 500: want error, got nil")
	}
}

func TestK6_ProgressReflectsRunning(t *testing.T) {
	t.Parallel()
	url := newAgent(t, &fakeAgent{})
	e := k6adapter.New(nil)
	ctx := context.Background()

	if running, _ := e.Progress(ctx, url); running {
		t.Fatal("progress before start: want not running")
	}
	if err := e.Trigger(ctx, url, engine.Config{}); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if running, err := e.Progress(ctx, url); err != nil || !running {
		t.Fatalf("progress after start: running=%v err=%v", running, err)
	}
	if err := e.Stop(ctx, url); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if running, _ := e.Progress(ctx, url); running {
		t.Fatal("progress after stop: want not running")
	}
}

func TestK6_SubscribeStreamError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	if _, err := k6adapter.New(nil).Subscribe(context.Background(), srv.URL); err == nil {
		t.Fatal("Subscribe on 500: want error, got nil")
	}
}

func TestK6_StopAndProgressServerErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	e := k6adapter.New(nil)
	ctx := context.Background()

	if err := e.Stop(ctx, srv.URL); err == nil {
		t.Fatal("Stop with 500: want error, got nil")
	}
	if _, err := e.Progress(ctx, srv.URL); err == nil {
		t.Fatal("Progress with 500: want error, got nil")
	}
}

func TestK6_RequestErrorsOnBadURL(t *testing.T) {
	t.Parallel()
	e := k6adapter.New(nil)
	ctx := context.Background()
	const bad = "http://127.0.0.1:0" // connect always fails

	if err := e.Trigger(ctx, bad, engine.Config{}); err == nil {
		t.Fatal("Trigger to dead addr: want error")
	}
	if err := e.Stop(ctx, bad); err == nil {
		t.Fatal("Stop to dead addr: want error")
	}
	if _, err := e.Progress(ctx, bad); err == nil {
		t.Fatal("Progress to dead addr: want error")
	}
	if _, err := e.Subscribe(ctx, bad); err == nil {
		t.Fatal("Subscribe to dead addr: want error")
	}
}
