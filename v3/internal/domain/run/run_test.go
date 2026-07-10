package run_test

import (
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/domain/execution"
	"github.com/heridotlife/Setagaya/v3/internal/domain/run"
)

func ec(plans ...execution.ExecutionPlan) execution.ExecutionCollection {
	return execution.ExecutionCollection{Tests: plans}
}

func TestDerivePhase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		deployed int
		running  bool
		want     run.Phase
	}{
		{0, false, run.PhaseIdle},
		{3, false, run.PhaseDeployed},
		{3, true, run.PhaseRunning},
		{0, true, run.PhaseRunning}, // running dominates
	}
	for _, c := range cases {
		if got := run.DerivePhase(c.deployed, c.running); got != c.want {
			t.Errorf("DerivePhase(%d,%v) = %q, want %q", c.deployed, c.running, got, c.want)
		}
	}
}

func TestCanDeploy(t *testing.T) {
	t.Parallel()

	if err := run.CanDeploy(run.PhaseIdle); err != nil {
		t.Errorf("idle: %v", err)
	}
	if err := run.CanDeploy(run.PhaseDeployed); err != nil {
		t.Errorf("deployed: %v", err)
	}
	if err := run.CanDeploy(run.PhaseRunning); !errors.Is(err, run.ErrAlreadyRunning) {
		t.Errorf("running: err = %v, want ErrAlreadyRunning", err)
	}
}

func TestCanTrigger(t *testing.T) {
	t.Parallel()

	plan := execution.ExecutionPlan{PlanID: 1, Concurrency: 5, Rampup: 1, Engines: 2, Duration: 10}
	full := ec(plan)

	cases := []struct {
		name    string
		phase   run.Phase
		coll    execution.ExecutionCollection
		ready   int
		wantErr error
	}{
		{"no plans", run.PhaseDeployed, ec(), 0, run.ErrNoPlans},
		{"not deployed", run.PhaseIdle, full, 0, run.ErrNotDeployed},
		{"already running", run.PhaseRunning, full, 2, run.ErrAlreadyRunning},
		{"engines not ready", run.PhaseDeployed, full, 1, run.ErrEnginesNotReady},
		{"ok", run.PhaseDeployed, full, 2, nil},
		{"ok over-ready", run.PhaseDeployed, full, 3, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := run.CanTrigger(c.phase, c.coll, c.ready)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestCanStop(t *testing.T) {
	t.Parallel()

	if err := run.CanStop(run.PhaseRunning); err != nil {
		t.Errorf("running: %v", err)
	}
	for _, p := range []run.Phase{run.PhaseIdle, run.PhaseDeployed} {
		if err := run.CanStop(p); !errors.Is(err, run.ErrNotRunning) {
			t.Errorf("%s: err = %v, want ErrNotRunning", p, err)
		}
	}
}

func TestVirtualUsers(t *testing.T) {
	t.Parallel()

	got := run.VirtualUsers(ec(
		execution.ExecutionPlan{Engines: 2, Concurrency: 10},
		execution.ExecutionPlan{Engines: 3, Concurrency: 5},
	))
	if got != 2*10+3*5 {
		t.Errorf("VirtualUsers = %d, want %d", got, 2*10+3*5)
	}
	if got := run.VirtualUsers(ec()); got != 0 {
		t.Errorf("empty VirtualUsers = %d, want 0", got)
	}
}

func TestRun_FinishedAndElapsed(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	open := run.Run{ID: 1, CollectionID: 2, StartedAt: start}
	if open.Finished() {
		t.Error("open run should not be finished")
	}
	now := start.Add(90 * time.Second)
	if got := open.Elapsed(now); got != 90*time.Second {
		t.Errorf("open Elapsed = %v, want 90s", got)
	}

	end := start.Add(30 * time.Second)
	done := run.Run{ID: 1, StartedAt: start, EndedAt: &end}
	if !done.Finished() {
		t.Error("closed run should be finished")
	}
	if got := done.Elapsed(now); got != 30*time.Second {
		t.Errorf("done Elapsed = %v, want 30s (ignores now)", got)
	}
}
