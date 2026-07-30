package metricsapp_test

import (
	"context"
	"errors"
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
	exec        *fake.Executor
	sink        *fake.MetricsSink
	bus         *membus.Bus
	executionID int64
	planIDs     []int64
	runID       int64
}

func setup(t *testing.T, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()
	coll, _ := execution.New("peak", 1)
	executionID, _ := store.CreateExecution(ctx, coll)

	var tests []loadprofile.Entry
	var planIDs []int64
	sched := fake.NewScheduler()
	for _, n := range engines {
		pl, _ := scenario.New("p", 1)
		scenarioID, _ := store.CreateScenario(ctx, pl)
		planIDs = append(planIDs, scenarioID)
		tests = append(tests, loadprofile.Entry{ScenarioID: scenarioID, Concurrency: 1, Rampup: 1, Engines: n, Duration: 1})
		_ = sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 1, ExecutionID: executionID, ScenarioID: scenarioID, Engines: n})
	}
	_ = store.StoreLoadProfile(ctx, executionID, false, tests)
	runID, _ := store.StartRun(ctx, executionID)

	exec := fake.NewExecutor()
	exec.Metrics = []engine.Metric{{Label: "a", Latency: 1}, {Label: "b", Latency: 2}}
	sink := fake.NewMetricsSink()
	bus := membus.New()
	svc := metricsapp.NewService(store, sched, exec, sink, bus)
	return &env{svc: svc, store: store, sched: sched, exec: exec, sink: sink, bus: bus, executionID: executionID, planIDs: planIDs, runID: runID}
}

func TestCollectScenario_EnrichesAndFansOut(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	ctx := context.Background()

	if err := e.svc.CollectScenario(ctx, e.executionID, e.planIDs[0], 2, e.runID); err != nil {
		t.Fatalf("CollectScenario: %v", err)
	}
	got := e.sink.Recorded()
	// 2 engines x 2 metrics.
	if len(got) != 4 {
		t.Fatalf("recorded = %d, want 4", len(got))
	}
	for _, m := range got {
		if m.ExecutionID == "" || m.ScenarioID == "" || m.EngineID == "" || m.RunID == "" {
			t.Fatalf("metric not enriched: %+v", m)
		}
	}
}

func TestCollectScenario_UnreachableErrors(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	e.sched.Unreachable = true
	if err := e.svc.CollectScenario(context.Background(), e.executionID, e.planIDs[0], 2, e.runID); err == nil {
		t.Fatal("CollectScenario unreachable: want error")
	}
}

func TestCollectExecution_AllScenarios(t *testing.T) {
	t.Parallel()
	e := setup(t, 2, 3)
	if err := e.svc.CollectExecution(context.Background(), e.executionID); err != nil {
		t.Fatalf("CollectExecution: %v", err)
	}
	// (2+3) engines x 2 metrics.
	if got := len(e.sink.Recorded()); got != 10 {
		t.Fatalf("recorded = %d, want 10", got)
	}
}

func TestCollectExecution_PropagatesScenarioError(t *testing.T) {
	t.Parallel()
	e := setup(t, 2, 3)
	e.sched.Unreachable = true // every scenario's EngineURLs fails
	if err := e.svc.CollectExecution(context.Background(), e.executionID); err == nil {
		t.Fatal("CollectExecution with unreachable engines: want error, got nil")
	}
}

func TestCollectExecution_NoActiveRunIsNoop(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	if err := e.store.StopRun(context.Background(), e.executionID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	if err := e.svc.CollectExecution(context.Background(), e.executionID); err != nil {
		t.Fatalf("CollectExecution: %v", err)
	}
	if got := len(e.sink.Recorded()); got != 0 {
		t.Fatalf("recorded = %d, want 0 (no active run)", got)
	}
}

func TestStartPublishesToBusThenStop(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	events, cancel := e.bus.Subscribe(e.executionID)
	defer cancel()

	e.svc.Start(e.executionID)
	select {
	case m := <-events:
		if m.ExecutionID == "" {
			t.Fatalf("bus metric not enriched: %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metric published to bus after Start")
	}
	// Start is idempotent and Stop is safe.
	e.svc.Start(e.executionID)
	e.svc.Stop(e.executionID)
}

func TestPurgeStopsAndDropsSeries(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	e.svc.Purge(e.executionID)
	if got := e.sink.Deleted(); len(got) != 1 || got[0] != e.executionID {
		t.Fatalf("deleted = %+v, want [%d]", got, e.executionID)
	}
}

func TestPumpEngine_SubscribeErrorRecordsNothing(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	e.exec.SubscribeErr = errors.New("stream down")
	// CollectScenario still returns nil (per-engine subscribe failures are skipped),
	// but nothing is recorded.
	if err := e.svc.CollectScenario(context.Background(), e.executionID, e.planIDs[0], 2, e.runID); err != nil {
		t.Fatalf("CollectScenario: %v", err)
	}
	if got := len(e.sink.Recorded()); got != 0 {
		t.Fatalf("recorded = %d, want 0 on subscribe error", got)
	}
}

func TestResume_StartsRunningExecutions(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()
	if err := e.store.MarkScenarioRunning(ctx, e.executionID, e.planIDs[0]); err != nil {
		t.Fatalf("MarkScenarioRunning: %v", err)
	}
	events, cancel := e.bus.Subscribe(e.executionID)
	defer cancel()

	if err := e.svc.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("Resume did not start execution")
	}
}
