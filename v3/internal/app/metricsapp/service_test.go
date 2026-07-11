package metricsapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	membus "github.com/heridotlife/Setagaya/v3/internal/adapters/eventbus/memory"
	"github.com/heridotlife/Setagaya/v3/internal/app/metricsapp"
	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/domain/engine"
	"github.com/heridotlife/Setagaya/v3/internal/domain/execution"
	"github.com/heridotlife/Setagaya/v3/internal/domain/plan"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
	"github.com/heridotlife/Setagaya/v3/internal/ports/fake"
)

type env struct {
	svc          *metricsapp.Service
	store        *fake.Store
	sched        *fake.Scheduler
	exec         *fake.Executor
	sink         *fake.MetricsSink
	bus          *membus.Bus
	collectionID int64
	planIDs      []int64
	runID        int64
}

func setup(t *testing.T, engines ...int) *env {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()
	coll, _ := collection.New("peak", 1)
	collectionID, _ := store.CreateCollection(ctx, coll)

	var tests []execution.ExecutionPlan
	var planIDs []int64
	sched := fake.NewScheduler()
	for _, n := range engines {
		pl, _ := plan.New("p", 1)
		planID, _ := store.CreatePlan(ctx, pl)
		planIDs = append(planIDs, planID)
		tests = append(tests, execution.ExecutionPlan{PlanID: planID, Concurrency: 1, Rampup: 1, Engines: n, Duration: 1})
		_ = sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: 1, CollectionID: collectionID, PlanID: planID, Engines: n})
	}
	_ = store.StoreExecutionCollection(ctx, collectionID, false, tests)
	runID, _ := store.StartRun(ctx, collectionID)

	exec := fake.NewExecutor()
	exec.Metrics = []engine.Metric{{Label: "a", Latency: 1}, {Label: "b", Latency: 2}}
	sink := fake.NewMetricsSink()
	bus := membus.New()
	svc := metricsapp.NewService(store, sched, exec, sink, bus)
	return &env{svc: svc, store: store, sched: sched, exec: exec, sink: sink, bus: bus, collectionID: collectionID, planIDs: planIDs, runID: runID}
}

func TestCollectPlan_EnrichesAndFansOut(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	ctx := context.Background()

	if err := e.svc.CollectPlan(ctx, e.collectionID, e.planIDs[0], 2, e.runID); err != nil {
		t.Fatalf("CollectPlan: %v", err)
	}
	got := e.sink.Recorded()
	// 2 engines x 2 metrics.
	if len(got) != 4 {
		t.Fatalf("recorded = %d, want 4", len(got))
	}
	for _, m := range got {
		if m.CollectionID == "" || m.PlanID == "" || m.EngineID == "" || m.RunID == "" {
			t.Fatalf("metric not enriched: %+v", m)
		}
	}
}

func TestCollectPlan_UnreachableErrors(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	e.sched.Unreachable = true
	if err := e.svc.CollectPlan(context.Background(), e.collectionID, e.planIDs[0], 2, e.runID); err == nil {
		t.Fatal("CollectPlan unreachable: want error")
	}
}

func TestCollectCollection_AllPlans(t *testing.T) {
	t.Parallel()
	e := setup(t, 2, 3)
	if err := e.svc.CollectCollection(context.Background(), e.collectionID); err != nil {
		t.Fatalf("CollectCollection: %v", err)
	}
	// (2+3) engines x 2 metrics.
	if got := len(e.sink.Recorded()); got != 10 {
		t.Fatalf("recorded = %d, want 10", got)
	}
}

func TestCollectCollection_PropagatesPlanError(t *testing.T) {
	t.Parallel()
	e := setup(t, 2, 3)
	e.sched.Unreachable = true // every plan's EngineURLs fails
	if err := e.svc.CollectCollection(context.Background(), e.collectionID); err == nil {
		t.Fatal("CollectCollection with unreachable engines: want error, got nil")
	}
}

func TestCollectCollection_NoActiveRunIsNoop(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	if err := e.store.StopRun(context.Background(), e.collectionID); err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	if err := e.svc.CollectCollection(context.Background(), e.collectionID); err != nil {
		t.Fatalf("CollectCollection: %v", err)
	}
	if got := len(e.sink.Recorded()); got != 0 {
		t.Fatalf("recorded = %d, want 0 (no active run)", got)
	}
}

func TestStartPublishesToBusThenStop(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	events, cancel := e.bus.Subscribe(e.collectionID)
	defer cancel()

	e.svc.Start(e.collectionID)
	select {
	case m := <-events:
		if m.CollectionID == "" {
			t.Fatalf("bus metric not enriched: %+v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metric published to bus after Start")
	}
	// Start is idempotent and Stop is safe.
	e.svc.Start(e.collectionID)
	e.svc.Stop(e.collectionID)
}

func TestPurgeStopsAndDropsSeries(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	e.svc.Purge(e.collectionID)
	if got := e.sink.Deleted(); len(got) != 1 || got[0] != e.collectionID {
		t.Fatalf("deleted = %+v, want [%d]", got, e.collectionID)
	}
}

func TestPumpEngine_SubscribeErrorRecordsNothing(t *testing.T) {
	t.Parallel()
	e := setup(t, 2)
	e.exec.SubscribeErr = errors.New("stream down")
	// CollectPlan still returns nil (per-engine subscribe failures are skipped),
	// but nothing is recorded.
	if err := e.svc.CollectPlan(context.Background(), e.collectionID, e.planIDs[0], 2, e.runID); err != nil {
		t.Fatalf("CollectPlan: %v", err)
	}
	if got := len(e.sink.Recorded()); got != 0 {
		t.Fatalf("recorded = %d, want 0 on subscribe error", got)
	}
}

func TestResume_StartsRunningCollections(t *testing.T) {
	t.Parallel()
	e := setup(t, 1)
	ctx := context.Background()
	if err := e.store.MarkPlanRunning(ctx, e.collectionID, e.planIDs[0]); err != nil {
		t.Fatalf("MarkPlanRunning: %v", err)
	}
	events, cancel := e.bus.Subscribe(e.collectionID)
	defer cancel()

	if err := e.svc.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("Resume did not start collection")
	}
}
