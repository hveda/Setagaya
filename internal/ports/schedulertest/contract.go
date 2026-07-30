// Package schedulertest holds the shared behavioural contract that every
// ports.Scheduler implementation must satisfy. The same suite runs against the
// in-memory fake and the real k8s adapter (over a fake clientset).
package schedulertest

import (
	"context"
	"testing"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// Harness wires a Scheduler under test together with a Ready hook that
// simulates the platform bringing a plan's engines up (pods ready, routable).
// Real clusters do this on their own; the fake and fake-clientset adapters use
// the hook to reach a deterministic ready state.
type Harness struct {
	Scheduler ports.Scheduler
	Ready     func(executionID, planID int64, engines int)
}

// NewHarness builds a fresh Harness for one test.
type NewHarness func(t *testing.T) Harness

// RunSchedulerContract exercises the full deploy → ready → query → purge cycle.
func RunSchedulerContract(t *testing.T, newHarness NewHarness) {
	t.Helper()
	ctx := context.Background()
	const (
		project    = int64(1)
		collection = int64(7)
		planA      = int64(10)
		planB      = int64(11)
	)

	t.Run("deploy status urls detail purge", func(t *testing.T) {
		h := newHarness(t)
		s := h.Scheduler

		mustDeploy(t, s, ports.DeploySpec{ProjectID: project, ExecutionID: collection, PlanID: planA, Engines: 2, Image: "jmeter"})
		mustDeploy(t, s, ports.DeploySpec{ProjectID: project, ExecutionID: collection, PlanID: planB, Engines: 3, Image: "jmeter"})

		deployed, err := s.DeployedCollections(ctx)
		if err != nil {
			t.Fatalf("DeployedCollections: %v", err)
		}
		if _, ok := deployed[collection]; !ok {
			t.Fatalf("collection %d not reported deployed: %v", collection, deployed)
		}

		h.Ready(collection, planA, 2)
		h.Ready(collection, planB, 3)

		urls, err := s.EngineURLs(ctx, collection, planA, 2)
		if err != nil {
			t.Fatalf("EngineURLs: %v", err)
		}
		if len(urls) != 2 {
			t.Fatalf("EngineURLs len = %d, want 2 (%v)", len(urls), urls)
		}

		status, err := s.CollectionStatus(ctx, collection, []ports.PlanRef{{PlanID: planA, Engines: 2}, {PlanID: planB, Engines: 3}})
		if err != nil {
			t.Fatalf("CollectionStatus: %v", err)
		}
		if len(status.Plans) != 2 {
			t.Fatalf("status plans = %d, want 2", len(status.Plans))
		}
		byPlan := map[int64]ports.PlanReadiness{}
		for _, p := range status.Plans {
			byPlan[p.PlanID] = p
		}
		if got := byPlan[planA]; got.EnginesWanted != 2 || got.EnginesDeployed != 2 || !got.Reachable {
			t.Fatalf("planA readiness = %+v, want wanted=2 deployed=2 reachable", got)
		}
		if got := byPlan[planB]; got.EnginesWanted != 3 || got.EnginesDeployed != 3 {
			t.Fatalf("planB readiness = %+v, want wanted=3 deployed=3", got)
		}
		if status.PoolSize != 5 {
			t.Fatalf("PoolSize = %d, want 5", status.PoolSize)
		}

		detail, err := s.EngineDetail(ctx, project, collection)
		if err != nil {
			t.Fatalf("EngineDetail: %v", err)
		}
		if len(detail.Engines) != 5 {
			t.Fatalf("detail engines = %d, want 5", len(detail.Engines))
		}

		if _, err := s.PodLog(ctx, collection, planA); err != nil {
			t.Fatalf("PodLog: %v", err)
		}

		if err := s.PurgeCollection(ctx, collection); err != nil {
			t.Fatalf("PurgeCollection: %v", err)
		}
		deployed, err = s.DeployedCollections(ctx)
		if err != nil {
			t.Fatalf("DeployedCollections after purge: %v", err)
		}
		if _, ok := deployed[collection]; ok {
			t.Fatalf("collection still deployed after purge: %v", deployed)
		}
		if _, err := s.EngineURLs(ctx, collection, planA, 2); err == nil {
			t.Fatalf("EngineURLs after purge: want error, got nil")
		}
	})

	t.Run("purge with nothing deployed is not an error", func(t *testing.T) {
		h := newHarness(t)
		if err := h.Scheduler.PurgeCollection(ctx, 999); err != nil {
			t.Fatalf("purge empty: %v", err)
		}
	})
}

func mustDeploy(t *testing.T, s ports.Scheduler, spec ports.DeploySpec) {
	t.Helper()
	if err := s.DeployPlan(context.Background(), spec); err != nil {
		t.Fatalf("DeployPlan(plan %d): %v", spec.PlanID, err)
	}
}
