package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newReportEnv(t *testing.T) (http.Handler, *fake.ReportStore, *fake.ObjectStore) {
	t.Helper()
	reports := fake.NewReportStore()
	obj := fake.NewObjectStore()
	h := httpapi.NewRouter(httpapi.Deps{Reports: reports, Store: obj, DefaultOwners: []string{"honryu"}})
	return h, reports, obj
}

func sampleReport(executionID, runID int64) report.Report {
	return report.Build(report.Input{
		ExecutionID: executionID, RunID: runID,
		Engine:    taurus.ExecutorJMeter,
		StartedAt: time.Unix(1000, 0).UTC(),
		EndedAt:   time.Unix(1030, 0).UTC(),
		Outcome:   taurus.OutcomeFailed,
		Requested: report.Load{Concurrency: 10, DurationSeconds: 30},
		Intervals: []metrics.Interval{{
			Timestamp: 1000, Label: "checkout", Samples: 10, Failed: 3, Succeeded: 7,
			Latency: metrics.Histogram{0.01: 7, 0.5: 3},
			Errors:  []metrics.ErrorGroup{{Message: "Not Found", ResponseCode: "404", Count: 3}},
		}},
	})
}

func TestReportHTTP_FetchesAStoredReport(t *testing.T) {
	t.Parallel()
	h, reports, _ := newReportEnv(t)
	want := sampleReport(1, 42)
	if err := reports.SaveReport(context.Background(), want); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/runs/42/report")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET report = %d (%s)", rec.Code, rec.Body.String())
	}
	var got report.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RunID != 42 || got.Outcome != taurus.OutcomeFailed {
		t.Errorf("report = %+v", got)
	}
	if len(got.Errors) != 1 || got.Errors[0].Count != 3 {
		t.Errorf("errors = %+v, want one signature counted 3", got.Errors)
	}
}

func TestReportHTTP_UnknownRunIs404(t *testing.T) {
	t.Parallel()
	h, _, _ := newReportEnv(t)

	if rec := do(t, h, http.MethodGet, "/api/runs/999/report"); rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown report = %d, want 404", rec.Code)
	}
}

func TestReportHTTP_ListsAnExecutionsReportsMostRecentFirst(t *testing.T) {
	t.Parallel()
	h, reports, _ := newReportEnv(t)
	ctx := context.Background()

	older := sampleReport(7, 1)
	older.StartedAt = time.Unix(1000, 0).UTC()
	newer := sampleReport(7, 2)
	newer.StartedAt = time.Unix(2000, 0).UTC()
	if err := reports.SaveReport(ctx, older); err != nil {
		t.Fatalf("SaveReport(older): %v", err)
	}
	if err := reports.SaveReport(ctx, newer); err != nil {
		t.Fatalf("SaveReport(newer): %v", err)
	}
	// A different execution's report must not appear.
	if err := reports.SaveReport(ctx, sampleReport(9, 3)); err != nil {
		t.Fatalf("SaveReport(other execution): %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/executions/7/reports")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET reports = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []report.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("reports = %d, want 2", len(got))
	}
	if got[0].RunID != 2 || got[1].RunID != 1 {
		t.Fatalf("reports = %+v, want run 2 before run 1", got)
	}
}

func TestReportHTTP_ListLimitIsRespected(t *testing.T) {
	t.Parallel()
	h, reports, _ := newReportEnv(t)
	ctx := context.Background()
	for i, ts := range []int64{1000, 2000, 3000} {
		r := sampleReport(7, int64(i+1))
		r.StartedAt = time.Unix(ts, 0).UTC()
		if err := reports.SaveReport(ctx, r); err != nil {
			t.Fatalf("SaveReport: %v", err)
		}
	}

	rec := do(t, h, http.MethodGet, "/api/executions/7/reports?limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET reports = %d", rec.Code)
	}
	var got []report.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].RunID != 3 {
		t.Fatalf("reports = %+v, want only the most recent (run 3)", got)
	}
}

// A malformed limit degrades to "no limit" rather than rejecting an otherwise
// valid request over a hint.
func TestReportHTTP_MalformedLimitIsIgnored(t *testing.T) {
	t.Parallel()
	h, reports, _ := newReportEnv(t)
	if err := reports.SaveReport(context.Background(), sampleReport(7, 1)); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/executions/7/reports?limit=not-a-number")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET reports = %d", rec.Code)
	}
	var got []report.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("reports = %+v, want the one report despite the bad limit", got)
	}
}

func TestReportHTTP_FetchesACapturedShardLog(t *testing.T) {
	t.Parallel()
	h, _, obj := newReportEnv(t)
	if err := obj.Upload(context.Background(), "run/42/scenario-3-shard-1.log", strings.NewReader("boom: 500")); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/runs/42/scenarios/3/shards/1/log")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET log = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "boom: 500" {
		t.Errorf("log body = %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

func TestReportHTTP_FetchesTheSnapshottedShardConfig(t *testing.T) {
	t.Parallel()
	h, _, obj := newReportEnv(t)
	if err := obj.Upload(context.Background(), "run/42/scenario-3-shard-1.yml", strings.NewReader("execution: []")); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/runs/42/scenarios/3/shards/1/config")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET config = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "execution: []" {
		t.Errorf("config body = %q", got)
	}
}

func TestReportHTTP_UncapturedShardObjectsAre404(t *testing.T) {
	t.Parallel()
	h, _, _ := newReportEnv(t)

	for _, path := range []string{
		"/api/runs/1/scenarios/1/shards/0/log",
		"/api/runs/1/scenarios/1/shards/0/config",
	} {
		if rec := do(t, h, http.MethodGet, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestReportHTTP_InvalidPathParamsAreBadRequest(t *testing.T) {
	t.Parallel()
	h, _, _ := newReportEnv(t)

	for _, path := range []string{
		"/api/runs/not-a-number/report",
		"/api/executions/not-a-number/reports",
		"/api/runs/not-a-number/scenarios/1/shards/0/log",
		"/api/runs/1/scenarios/not-a-number/shards/0/log",
		"/api/runs/1/scenarios/1/shards/not-a-number/log",
	} {
		if rec := do(t, h, http.MethodGet, path); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, rec.Code)
		}
	}
}
