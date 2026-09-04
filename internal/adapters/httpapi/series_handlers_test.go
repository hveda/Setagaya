package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// newSeriesEnv wires the two collaborators the series endpoint needs: the
// report store (existence and authorization) and the interval repository the
// same fake Absorb writes, so a test seeds the series exactly the way the real
// ingest path produced it.
func newSeriesEnv(t *testing.T) (http.Handler, *fake.ReportStore, *fake.ReportProgress) {
	t.Helper()
	reports := fake.NewReportStore()
	progress := fake.NewReportProgress()
	h := httpapi.NewRouter(httpapi.Deps{
		Reports: reports, Series: progress,
		Store: fake.NewObjectStore(), DefaultOwners: []string{"honryu"},
	})
	return h, reports, progress
}

func seriesBatch(runID int64, shard int, final bool, intervals ...metrics.Interval) ports.ProgressBatch {
	return ports.ProgressBatch{
		RunID: runID, ScenarioID: 1, ShardIndex: shard, StreamID: "s1", Final: final,
		Intervals: intervals,
	}
}

func TestSeriesHTTP_ServesAMergedRun(t *testing.T) {
	t.Parallel()
	h, reports, progress := newSeriesEnv(t)
	if err := reports.SaveReport(context.Background(), sampleReport(1, 42)); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}
	// Two shards, two seconds: what the endpoint merges is already pod-merged
	// by ingest, so the chart sees one point per second.
	mustAbsorb(t, progress, seriesBatch(42, 0, true,
		metrics.Interval{
			Seq: 1, Timestamp: 1000, Label: "checkout", Concurrency: 5,
			Samples: 10, Succeeded: 9, Failed: 1, Latency: metrics.Histogram{0.01: 9, 0.2: 1},
		},
		metrics.Interval{
			Seq: 2, Timestamp: 1001, Label: "checkout", Concurrency: 5,
			Samples: 8, Succeeded: 8, Latency: metrics.Histogram{0.01: 8},
		},
	))
	mustAbsorb(t, progress, seriesBatch(42, 1, true,
		metrics.Interval{
			Seq: 1, Timestamp: 1000, Label: "checkout", Concurrency: 4,
			Samples: 5, Succeeded: 4, Failed: 1, Latency: metrics.Histogram{0.01: 5},
		},
	))

	rec := do(t, h, http.MethodGet, "/api/runs/42/series")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET series = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Points []struct {
			Ts      int64              `json:"ts"`
			VUs     float64            `json:"vus"`
			RPS     float64            `json:"rps"`
			ErrPct  float64            `json:"err_pct"`
			Latency map[string]float64 `json:"latency"`
		} `json:"points"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Points) != 2 {
		t.Fatalf("points = %+v, want 1000 and 1001", got.Points)
	}
	first := got.Points[0]
	if first.Ts != 1000 || first.VUs != 9 || first.RPS != 15 {
		t.Errorf("first point = %+v, want ts 1000, vus 9, rps 15 (both shards summed)", first)
	}
	if want := float64(2) / 15 * 100; first.ErrPct != want {
		t.Errorf("err_pct = %v, want %v", first.ErrPct, want)
	}
	// 15 samples, buckets {0.01: 14, 0.2: 1}: nearest-rank p50 is 0.01, p95
	// (rank 15) lands in the tail bucket. Latency is in seconds.
	if first.Latency["50"] != 0.01 || first.Latency["95"] != 0.2 || first.Latency["99"] != 0.2 {
		t.Errorf("latency = %v, want p50 0.01, p95/p99 0.2 (seconds)", first.Latency)
	}
	if got.Points[1].Ts != 1001 {
		t.Errorf("second point ts = %d, want 1001", got.Points[1].Ts)
	}
}

func TestSeriesHTTP_EmptyRunIsAnEmptyPointsArray(t *testing.T) {
	t.Parallel()
	h, reports, _ := newSeriesEnv(t)
	if err := reports.SaveReport(context.Background(), sampleReport(1, 42)); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/runs/42/series")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET series = %d (%s)", rec.Code, rec.Body.String())
	}
	// An empty run charts nothing -- an empty array, not null, so a client's
	// .points.map() never has to guard.
	if strings.TrimSpace(rec.Body.String()) != `{"points":[]}` {
		t.Fatalf("body = %s, want {\"points\":[]}", rec.Body.String())
	}
}

// The unknown-run path is runReport's: the stored report is what makes a run
// known, so its absence is a 404 for the series too.
func TestSeriesHTTP_UnknownRunIs404(t *testing.T) {
	t.Parallel()
	h, _, _ := newSeriesEnv(t)
	rec := do(t, h, http.MethodGet, "/api/runs/999/series")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET series (unknown run) = %d, want 404", rec.Code)
	}
}

func TestSeriesHTTP_InvalidRunIDIsBadRequest(t *testing.T) {
	t.Parallel()
	h, _, _ := newSeriesEnv(t)
	rec := do(t, h, http.MethodGet, "/api/runs/abc/series")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET series (bad id) = %d, want 400", rec.Code)
	}
}

// A report-store-only deployment has no interval store wired: the route
// answers 404 rather than panicking, the optional-dependency precedent.
func TestSeriesHTTP_UnwiredSeriesIs404(t *testing.T) {
	t.Parallel()
	h := httpapi.NewRouter(httpapi.Deps{
		Reports: fake.NewReportStore(), Store: fake.NewObjectStore(),
		DefaultOwners: []string{"honryu"},
	})
	rec := do(t, h, http.MethodGet, "/api/runs/42/series")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET series (unwired) = %d, want 404", rec.Code)
	}
}

func mustAbsorb(t *testing.T, p *fake.ReportProgress, b ports.ProgressBatch) {
	t.Helper()
	if err := p.Absorb(context.Background(), b); err != nil {
		t.Fatalf("Absorb: %v", err)
	}
}
