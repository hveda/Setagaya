package adminapp_test

import (
	"context"
	"testing"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/app/adminapp"
	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
	"github.com/heridotlife/Setagaya/v3/internal/ports/fake"
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
	c, _ := collection.New("peak", 3)
	collectionID, _ := store.CreateCollection(ctx, c)
	sched := fake.NewScheduler()
	purger := &recordingPurger{}
	svc := adminapp.NewService(store, sched, purger)
	return store, sched, purger, svc, collectionID
}

func TestRunningCollections_Enriched(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, sched, _, svc, collectionID := seed(t)
	_ = store
	if err := sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: 3, CollectionID: collectionID, PlanID: 1, Engines: 2}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	list, err := svc.RunningCollections(ctx)
	if err != nil {
		t.Fatalf("RunningCollections: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("running = %d, want 1", len(list))
	}
	if list[0].Name != "peak" || list[0].ProjectID != 3 || list[0].Running {
		t.Fatalf("running collection = %+v", list[0])
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
	_, sched, purger, svc, collectionID := seed(t)
	// Engines deployed two hours ago.
	sched.Now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	if err := sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: 3, CollectionID: collectionID, PlanID: 1, Engines: 1}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	purged, err := svc.AutoPurgeStale(ctx, time.Hour)
	if err != nil {
		t.Fatalf("AutoPurgeStale: %v", err)
	}
	if len(purged) != 1 || purged[0] != collectionID {
		t.Fatalf("purged = %v, want [%d]", purged, collectionID)
	}
	if len(purger.purged) != 1 {
		t.Fatalf("purger called %d times, want 1", len(purger.purged))
	}
}

func TestAutoPurgeStale_SkipsFreshAndRunning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, sched, purger, svc, collectionID := seed(t)

	// Fresh deployment (now): not stale.
	if err := sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: 3, CollectionID: collectionID, PlanID: 1, Engines: 1}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if purged, _ := svc.AutoPurgeStale(ctx, time.Hour); len(purged) != 0 {
		t.Fatalf("fresh purged = %v, want none", purged)
	}

	// Old but running: skipped.
	sched.Now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	c2, _ := collection.New("busy", 3)
	c2ID, _ := store.CreateCollection(ctx, c2)
	if err := sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: 3, CollectionID: c2ID, PlanID: 2, Engines: 1}); err != nil {
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
