package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

func TestFakeScheduler_Branches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := fake.NewScheduler()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return fixed }

	// Undeployed: URLs and pod logs are unavailable.
	if _, err := s.EngineURLs(ctx, 1, 2, 1); !errors.Is(err, ports.ErrEnginesUnreachable) {
		t.Fatalf("EngineURLs undeployed = %v", err)
	}
	if _, err := s.PodLog(ctx, 1, 2); err == nil {
		t.Fatal("PodLog undeployed: want error")
	}

	spec := ports.DeploySpec{ProjectID: 9, ExecutionID: 1, ScenarioID: 2, Engines: 2}
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	// Re-deploy keeps the original deploy time (idempotent).
	spec.Engines = 4
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("redeploy: %v", err)
	}
	deployed, _ := s.DeployedExecutions(ctx)
	if !deployed[1].Equal(fixed) {
		t.Fatalf("deploy time = %v, want %v", deployed[1], fixed)
	}

	log, err := s.PodLog(ctx, 1, 2)
	if err != nil || log == "" {
		t.Fatalf("PodLog = %q, %v", log, err)
	}
}

func TestFakeExecutor_MetricsAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := fake.NewExecutor()
	e.Running = true
	e.Metrics = []engine.Metric{{Label: "a", Latency: 1}, {Label: "b", Latency: 2}}

	if running, _ := e.Progress(ctx, "u"); !running {
		t.Fatal("Progress should report running")
	}

	ch, err := e.Subscribe(ctx, "u")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	var got int
	for range ch {
		got++
	}
	if got != 2 {
		t.Fatalf("metrics replayed = %d, want 2", got)
	}

	e.TriggerErr = errors.New("boom")
	if err := e.Trigger(ctx, "u", engine.Config{}); err == nil {
		t.Fatal("Trigger with TriggerErr: want error")
	}
	e.StopErr = errors.New("boom")
	if err := e.Stop(ctx, "u"); err == nil {
		t.Fatal("Stop with StopErr: want error")
	}
}
