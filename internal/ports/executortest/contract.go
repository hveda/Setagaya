// Package executortest holds the shared behavioural contract that every
// ports.Executor implementation must satisfy. The same suite runs against the
// in-memory fake and the real JMeter adapter (over an httptest agent).
package executortest

import (
	"context"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewExecutor builds a fresh Executor for one test, along with the base URL of
// an engine it can drive.
type NewExecutor func(t *testing.T) (exec ports.Executor, engineURL string)

// RunExecutorContract exercises trigger → progress → subscribe → stop.
func RunExecutorContract(t *testing.T, newExec NewExecutor) {
	t.Helper()
	ctx := context.Background()

	t.Run("kind is non-empty", func(t *testing.T) {
		e, _ := newExec(t)
		if e.Kind() == "" {
			t.Fatal("Kind() is empty")
		}
	})

	t.Run("trigger then stop", func(t *testing.T) {
		e, url := newExec(t)
		cfg := engine.Config{
			Data:        map[string]engine.File{"p.jmx": {Filename: "p.jmx"}},
			Duration:    "60",
			Concurrency: "10",
			Rampup:      "5",
			RunID:       99,
			EngineID:    0,
		}
		if err := e.Trigger(ctx, url, cfg); err != nil {
			t.Fatalf("Trigger: %v", err)
		}
		// Triggering again is a no-op, not an error.
		if err := e.Trigger(ctx, url, cfg); err != nil {
			t.Fatalf("re-Trigger: %v", err)
		}
		if _, err := e.Progress(ctx, url); err != nil {
			t.Fatalf("Progress: %v", err)
		}
		if err := e.Stop(ctx, url); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	})

	t.Run("subscribe yields a closed channel", func(t *testing.T) {
		e, url := newExec(t)
		ch, err := e.Subscribe(ctx, url)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		// Drain until closed; the contract only requires the channel to close.
		for range ch { //nolint:revive // draining
		}
	})
}
