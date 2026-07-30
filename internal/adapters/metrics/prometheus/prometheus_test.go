package prometheus_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	promsink "github.com/heridotlife/Setagaya/internal/adapters/metrics/prometheus"
	"github.com/heridotlife/Setagaya/internal/domain/engine"
)

func metric(coll, plan, engineNo, run, label, status string, latency, threads float64) engine.Metric {
	return engine.Metric{
		ExecutionID: coll, PlanID: plan, EngineID: engineNo, RunID: run,
		Label: label, Status: status, Latency: latency, Threads: threads,
	}
}

func TestSink_RecordExposesSeries(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	sink := promsink.New(reg)

	sink.Record(metric("1", "10", "0", "99", "home", "200", 12.5, 8))
	sink.Record(metric("1", "10", "0", "99", "login", "500", 30, 8))

	// Two distinct status series (200, 500); one threads-gauge series (same
	// engine); one collection-latency series (same collection+run).
	if got := testutil.CollectAndCount(reg, "setagaya_status_counter"); got != 2 {
		t.Fatalf("status_counter series = %d, want 2", got)
	}
	if got := testutil.CollectAndCount(reg, "setagaya_threads_gauge"); got != 1 {
		t.Fatalf("threads_gauge series = %d, want 1", got)
	}
	if got := testutil.CollectAndCount(reg, "setagaya_latency_label"); got != 2 {
		t.Fatalf("latency_label series = %d, want 2", got)
	}
}

func TestSink_DeleteCollectionRemovesSeries(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	sink := promsink.New(reg)

	sink.Record(metric("1", "10", "0", "99", "home", "200", 12.5, 8))
	sink.Record(metric("2", "20", "0", "99", "home", "200", 5, 4))
	if got := testutil.CollectAndCount(reg, "setagaya_status_counter"); got != 2 {
		t.Fatalf("before delete = %d, want 2", got)
	}

	sink.DeleteCollection(1)
	if got := testutil.CollectAndCount(reg, "setagaya_status_counter"); got != 1 {
		t.Fatalf("after delete collection 1 = %d, want 1", got)
	}
}
