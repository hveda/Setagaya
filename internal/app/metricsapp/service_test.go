package metricsapp_test

import (
	"context"
	"testing"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

type env struct {
	svc         *metricsapp.Service
	store       *fake.Store
	sink        *fake.MetricsSink
	bus         *membus.Bus
	progress    *fake.ReportProgress
	reports     *fake.ReportStore
	executionID int64
	scenarioIDs []int64
	runID       int64
	// seq numbers intervals across every batch a test builds, mirroring a real
	// sidecar's stream rather than restarting at one on every call.
	seq int64
}

func setup(t *testing.T, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()
	exe, _ := execution.New("peak", 1)
	executionID, _ := store.CreateExecution(ctx, exe)

	var tests []loadprofile.Entry
	var scenarioIDs []int64
	for _, n := range engines {
		sc, _ := scenario.New("p", 1)
		scenarioID, _ := store.CreateScenario(ctx, sc)
		scenarioIDs = append(scenarioIDs, scenarioID)
		tests = append(tests, loadprofile.Entry{ScenarioID: scenarioID, Concurrency: 1, Rampup: 1, Engines: n, Duration: 1})
	}
	_ = store.StoreLoadProfile(ctx, executionID, false, tests)
	runID, _ := store.StartRun(ctx, executionID)

	sink := fake.NewMetricsSink()
	bus := membus.New()
	progress := fake.NewReportProgress()
	reports := fake.NewReportStore()
	svc := metricsapp.NewService(store, sink, bus, progress, reports)
	return &env{
		svc: svc, store: store, sink: sink, bus: bus, progress: progress, reports: reports,
		executionID: executionID, scenarioIDs: scenarioIDs, runID: runID,
	}
}

// Purge is the only lifecycle hook left: with measurements pushed there is no
// collection to start or stop, but an execution's series still have to go when
// its engines do, or a long-lived controller keeps every series it ever made.
func TestPurgeDropsSeries(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)

	e.svc.Purge(e.executionID)

	if deleted := e.sink.Deleted(); len(deleted) != 1 || deleted[0] != e.executionID {
		t.Errorf("Deleted() = %v, want [%d]", deleted, e.executionID)
	}
}

// Purging also releases what the execution absorbed, so a controller running for
// weeks does not hold the interval keys of every run it has seen.
func TestPurgeReleasesDeduplicationState(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()

	b := batch(e, 0, 1)
	if err := e.svc.Ingest(ctx, b); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	e.svc.Purge(e.executionID)

	// State was released, so the same interval is absorbed again rather than
	// suppressed -- the execution it belonged to is over.
	if err := e.svc.Ingest(ctx, b); err != nil {
		t.Fatalf("Ingest after purge: %v", err)
	}
	if got := len(e.sink.Recorded()); got != 2 {
		t.Errorf("sink recorded %d, want 2", got)
	}
}
