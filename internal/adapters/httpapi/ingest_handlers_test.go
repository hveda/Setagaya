package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/authapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

const engineToken = "engine-s3cret"

type ingestEnv struct {
	h           http.Handler
	sink        *fake.MetricsSink
	executionID int64
	scenarioID  int64
	runID       int64
}

func newIngestEnv(t *testing.T, deps func(*httpapi.Deps)) ingestEnv {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()

	exe, _ := execution.New("peak", 1)
	executionID, _ := store.CreateExecution(ctx, exe)
	sc, _ := scenario.New("probe", 1)
	scenarioID, _ := store.CreateScenario(ctx, sc)
	_ = store.StoreLoadProfile(ctx, executionID, false, []loadprofile.Entry{
		{ScenarioID: scenarioID, Concurrency: 1, Rampup: 1, Engines: 1, Duration: 10},
	})
	runID, _ := store.StartRun(ctx, executionID, "")

	sink := fake.NewMetricsSink()
	d := httpapi.Deps{
		Metrics:     metricsapp.NewService(store, sink, membus.New(), fake.NewReportProgress(), fake.NewReportStore()),
		IngestToken: engineToken,
	}
	if deps != nil {
		deps(&d)
	}
	return ingestEnv{h: httpapi.NewRouter(d), sink: sink, executionID: executionID, scenarioID: scenarioID, runID: runID}
}

func postBatch(t *testing.T, h http.Handler, token string, b metrics.Batch) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (e ingestEnv) batch() metrics.Batch {
	return metrics.Batch{
		ExecutionID: e.executionID, ScenarioID: e.scenarioID, RunID: e.runID,
		Intervals: []metrics.Interval{{
			Seq: 1, Timestamp: 1, Label: "probe", Concurrency: 2, Samples: 9, Succeeded: 9,
			Latency: metrics.Histogram{0.01: 9},
		}},
	}
}

func TestIngestHTTP_AcceptsABatch(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, nil)

	rec := postBatch(t, e.h, engineToken, e.batch())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := len(e.sink.Recorded()); got != 1 {
		t.Errorf("sink recorded %d measurements, want 1", got)
	}
}

func TestIngestHTTP_RejectsBadCredentials(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, nil)

	for _, tc := range []struct{ name, token string }{
		{"no token", ""},
		{"wrong token", "not-the-token"},
		{"almost right", engineToken + "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := postBatch(t, e.h, tc.token, e.batch())
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("ingest with %s = %d, want 401", tc.name, rec.Code)
			}
		})
	}
	if got := len(e.sink.Recorded()); got != 0 {
		t.Errorf("unauthenticated pushes contributed %d measurements", got)
	}
}

// A deployment that never configured a credential must reject pushes rather than
// accept anonymous ones.
func TestIngestHTTP_ClosedWhenNoTokenIsConfigured(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, func(d *httpapi.Deps) { d.IngestToken = "" })

	if rec := postBatch(t, e.h, "", e.batch()); rec.Code != http.StatusUnauthorized {
		t.Errorf("ingest with no configured token = %d, want 401", rec.Code)
	}
	if rec := postBatch(t, e.h, engineToken, e.batch()); rec.Code != http.StatusUnauthorized {
		t.Errorf("ingest with a guessed token = %d, want 401", rec.Code)
	}
}

// Engine pods hold no user account. With RBAC enabled the user middleware would
// reject every push before it reached the handler, which would silently stop all
// metrics the moment a deployment turned auth on.
func TestIngestHTTP_WorksWhenRBACIsEnabled(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, func(d *httpapi.Deps) {
		d.Auth = authapp.NewService(nil, nil, true)
	})

	rec := postBatch(t, e.h, engineToken, e.batch())
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest with RBAC enabled = %d (%s), want 202", rec.Code, rec.Body.String())
	}
}

func TestIngestHTTP_RejectsMalformedBodies(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Authorization", "Bearer "+engineToken)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", rec.Code)
	}

	// A batch that names no execution cannot be attributed to anything.
	if rec := postBatch(t, e.h, engineToken, metrics.Batch{}); rec.Code != http.StatusBadRequest {
		t.Errorf("batch with no execution = %d, want 400", rec.Code)
	}
}

// The pod retries a push it could not complete, so the same batch arrives twice.
func TestIngestHTTP_IsIdempotent(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, nil)

	for i := 0; i < 3; i++ {
		if rec := postBatch(t, e.h, engineToken, e.batch()); rec.Code != http.StatusAccepted {
			t.Fatalf("push %d = %d", i, rec.Code)
		}
	}
	if got := len(e.sink.Recorded()); got != 1 {
		t.Errorf("sink recorded %d measurements after three identical pushes, want 1", got)
	}
}

// A pod that outlived its run keeps pushing until it is killed. That is a state
// conflict, not a server fault: reporting 500 would fill the logs with errors
// that are nobody's bug and hide the real ones.
func TestIngestHTTP_StoppedRunIsAConflictNotAServerError(t *testing.T) {
	t.Parallel()
	e := newIngestEnv(t, nil)

	stale := e.batch()
	stale.RunID = e.runID + 42
	if rec := postBatch(t, e.h, engineToken, stale); rec.Code != http.StatusConflict {
		t.Errorf("push for a finished run = %d, want 409", rec.Code)
	}
}
