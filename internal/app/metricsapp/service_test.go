package metricsapp_test

import (
	"context"
	"testing"
	"time"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

type env struct {
	svc         *metricsapp.Service
	store       *fake.Store
	sched       *fake.Scheduler
	sink        *fake.MetricsSink
	bus         *membus.Bus
	executionID int64
	scenarioIDs []int64
	runID       int64
}

func setup(t *testing.T, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()
	exe, _ := execution.New("peak", 1)
	executionID, _ := store.CreateExecution(ctx, exe)

	var tests []loadprofile.Entry
	var scenarioIDs []int64
	sched := fake.NewScheduler()
	for _, n := range engines {
		sc, _ := scenario.New("p", 1)
		scenarioID, _ := store.CreateScenario(ctx, sc)
		scenarioIDs = append(scenarioIDs, scenarioID)
		tests = append(tests, loadprofile.Entry{ScenarioID: scenarioID, Concurrency: 1, Rampup: 1, Engines: n, Duration: 1})
		_ = sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 1, ExecutionID: executionID, ScenarioID: scenarioID, Engines: n})
	}
	_ = store.StoreLoadProfile(ctx, executionID, false, tests)
	runID, _ := store.StartRun(ctx, executionID)

	sink := fake.NewMetricsSink()
	bus := membus.New()
	svc := metricsapp.NewService(store, sched, sink, bus)
	return &env{
		svc: svc, store: store, sched: sched, sink: sink, bus: bus,
		executionID: executionID, scenarioIDs: scenarioIDs, runID: runID,
	}
}

// Record is the seam the ingest endpoint will call. Its job is to stamp a
// measurement with the identity of the engine that produced it: without that a
// sample cannot be attributed to an execution, scenario, engine, or run.
func TestRecord_StampsIdentityAndFansOut(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)

	sub, unsubscribe := e.bus.Subscribe(e.executionID)
	defer unsubscribe()

	e.svc.Record(e.executionID, e.scenarioIDs[0], 2, e.runID,
		engine.Metric{Label: "checkout-cart", Latency: 7})

	recorded := e.sink.Recorded()
	if len(recorded) != 1 {
		t.Fatalf("sink recorded %d measurements, want 1", len(recorded))
	}
	got := recorded[0]
	if got.Label != "checkout-cart" || got.Latency != 7 {
		t.Errorf("measurement altered in flight: %+v", got)
	}
	if got.ExecutionID == "" || got.ScenarioID == "" || got.EngineID != "2" || got.RunID == "" {
		t.Errorf("identity not stamped: %+v", got)
	}

	select {
	case m := <-sub:
		if m.Label != "checkout-cart" || m.EngineID != "2" {
			t.Errorf("bus received %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Error("nothing published to the bus")
	}
}

func TestCollectExecution_NoActiveRunIsNoop(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	if err := e.store.StopRun(context.Background(), e.executionID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	if err := e.svc.CollectExecution(context.Background(), e.executionID); err != nil {
		t.Errorf("CollectExecution with no active run = %v, want nil", err)
	}
}

func TestCollectExecution_ActiveRun(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	if err := e.svc.CollectExecution(context.Background(), e.executionID); err != nil {
		t.Errorf("CollectExecution = %v, want nil", err)
	}
}

func TestStartIsIdempotentAndStopIsSafeTwice(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)

	e.svc.Start(e.executionID)
	e.svc.Start(e.executionID) // second Start must be a no-op, not a second tracker
	e.svc.Stop(e.executionID)
	e.svc.Stop(e.executionID) // stopping an unstarted execution must not panic
}

func TestPurgeStopsAndDropsSeries(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)

	e.svc.Start(e.executionID)
	e.svc.Purge(e.executionID)

	if deleted := e.sink.Deleted(); len(deleted) != 1 || deleted[0] != e.executionID {
		t.Errorf("Deleted() = %v, want [%d]", deleted, e.executionID)
	}
}

func TestResume_TracksExecutionsWithRunningScenarios(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()
	if err := e.store.MarkScenarioRunning(ctx, e.executionID, e.scenarioIDs[0]); err != nil {
		t.Fatalf("MarkScenarioRunning: %v", err)
	}
	if err := e.svc.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}
}
