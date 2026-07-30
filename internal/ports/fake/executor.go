package fake

import (
	"context"
	"sync"

	"github.com/heridotlife/honryu/internal/domain/engine"
)

// Executor is an in-memory ports.Executor for fast use-case tests. It records
// triggers and stops and can replay a fixed set of metrics on Subscribe.
type Executor struct {
	mu       sync.Mutex
	triggers map[string]engine.Config // engineURL -> last config
	stopped  map[string]bool          // engineURL -> stopped

	// Running is what Progress reports for every engine.
	Running bool
	// Metrics is replayed (in order) by Subscribe, then the channel closes.
	Metrics []engine.Metric
	// Repeat, when true, makes Subscribe loop over Metrics until ctx is
	// cancelled instead of closing after one pass (for long-lived stream tests).
	Repeat bool
	// TriggerErr, when set, is returned by Trigger.
	TriggerErr error
	// StopErr, when set, is returned by Stop.
	StopErr error
	// SubscribeErr, when set, is returned by Subscribe.
	SubscribeErr error
}

// NewExecutor returns an empty in-memory Executor.
func NewExecutor() *Executor {
	return &Executor{triggers: map[string]engine.Config{}, stopped: map[string]bool{}}
}

// Kind identifies the fake tool.
func (e *Executor) Kind() string { return "fake" }

// Trigger records the config sent to engineURL.
func (e *Executor) Trigger(_ context.Context, engineURL string, cfg engine.Config) error {
	if e.TriggerErr != nil {
		return e.TriggerErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.triggers[engineURL] = cfg
	return nil
}

// Stop records a stop for engineURL.
func (e *Executor) Stop(_ context.Context, engineURL string) error {
	if e.StopErr != nil {
		return e.StopErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped[engineURL] = true
	return nil
}

// Progress reports the configured Running flag.
func (e *Executor) Progress(_ context.Context, _ string) (bool, error) {
	return e.Running, nil
}

// Subscribe replays Metrics then closes the channel, or loops until ctx is
// cancelled when Repeat is set.
func (e *Executor) Subscribe(ctx context.Context, _ string) (<-chan engine.Metric, error) {
	if e.SubscribeErr != nil {
		return nil, e.SubscribeErr
	}
	ch := make(chan engine.Metric)
	go func() {
		defer close(ch)
		for {
			for _, m := range e.Metrics {
				select {
				case <-ctx.Done():
					return
				case ch <- m:
				}
			}
			if !e.Repeat {
				return
			}
		}
	}()
	return ch, nil
}

// TriggeredConfig returns the config recorded for engineURL and whether it was
// triggered. For assertions in tests.
func (e *Executor) TriggeredConfig(engineURL string) (engine.Config, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg, ok := e.triggers[engineURL]
	return cfg, ok
}

// TriggerCount returns how many distinct engines were triggered.
func (e *Executor) TriggerCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.triggers)
}

// StopCount returns how many distinct engines were stopped.
func (e *Executor) StopCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.stopped)
}
