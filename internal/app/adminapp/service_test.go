package adminapp_test

import (
	"context"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

type recordingPurger struct{ purged []int64 }

func (p *recordingPurger) Purge(_ context.Context, id int64) error {
	p.purged = append(p.purged, id)
	return nil
}

func seed(t *testing.T) (*fake.Store, *fake.Scheduler, *recordingPurger, *adminapp.Service, int64) {
	t.Helper()
	ctx := context.Background()
	store := fake.NewStore()
	c, _ := execution.New("peak", 3)
	executionID, _ := store.CreateExecution(ctx, c)
	sched := fake.NewScheduler()
	purger := &recordingPurger{}
	svc := adminapp.NewService(store, sched, purger)
	return store, sched, purger, svc, executionID
}

func TestRunningExecutions_Enriched(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, sched, _, svc, executionID := seed(t)
	_ = store
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 3, ExecutionID: executionID, ScenarioID: 1, Shards: deployShards(2)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	list, err := svc.RunningExecutions(ctx)
	if err != nil {
		t.Fatalf("RunningExecutions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("running = %d, want 1", len(list))
	}
	if list[0].Name != "peak" || list[0].ProjectID != 3 || list[0].Running {
		t.Fatalf("running execution = %+v", list[0])
	}
}

func TestNodePools(t *testing.T) {
	t.Parallel()
	_, _, _, svc, _ := seed(t)
	pools, err := svc.NodePools(context.Background())
	if err != nil {
		t.Fatalf("NodePools: %v", err)
	}
	if len(pools) == 0 {
		t.Fatal("expected at least one node pool")
	}
}

func TestAutoPurgeStale_PurgesIdle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, sched, purger, svc, executionID := seed(t)
	// Engines deployed two hours ago.
	sched.Now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 3, ExecutionID: executionID, ScenarioID: 1, Shards: deployShards(1)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	purged, err := svc.AutoPurgeStale(ctx, time.Hour)
	if err != nil {
		t.Fatalf("AutoPurgeStale: %v", err)
	}
	if len(purged) != 1 || purged[0] != executionID {
		t.Fatalf("purged = %v, want [%d]", purged, executionID)
	}
	if len(purger.purged) != 1 {
		t.Fatalf("purger called %d times, want 1", len(purger.purged))
	}
}

func TestAutoPurgeStale_SkipsFreshAndRunning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, sched, purger, svc, executionID := seed(t)

	// Fresh deployment (now): not stale.
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 3, ExecutionID: executionID, ScenarioID: 1, Shards: deployShards(1)}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if purged, _ := svc.AutoPurgeStale(ctx, time.Hour); len(purged) != 0 {
		t.Fatalf("fresh purged = %v, want none", purged)
	}

	// Old but running: skipped.
	sched.Now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	c2, _ := execution.New("busy", 3)
	c2ID, _ := store.CreateExecution(ctx, c2)
	if err := sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 3, ExecutionID: c2ID, ScenarioID: 2, Shards: deployShards(1)}); err != nil {
		t.Fatalf("deploy c2: %v", err)
	}
	if _, err := store.StartRun(ctx, c2ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if purged, _ := svc.AutoPurgeStale(ctx, time.Hour); len(purged) != 0 {
		t.Fatalf("running purged = %v, want none", purged)
	}
	if len(purger.purged) != 0 {
		t.Fatalf("purger should not have been called")
	}
}

// deployShards builds n placeholder shard specs for a deploy.
func deployShards(n int) []ports.ShardSpec {
	out := make([]ports.ShardSpec, n)
	for i := range out {
		out[i] = ports.ShardSpec{Index: i, Concurrency: 1}
	}
	return out
}
