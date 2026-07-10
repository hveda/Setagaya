package ports

import (
	"context"

	"github.com/heridotlife/Setagaya/v3/internal/domain/engine"
)

// Executor drives a load-testing tool (JMeter first; k6/Gatling later) on a
// single engine reachable at a base URL. It is the seam that lets a second
// tool drop in without touching the lifecycle use-cases. Building the
// per-engine engine.Config is pure domain (engine.BuildConfigs), so the
// Executor only performs I/O against the engine's agent.
type Executor interface {
	// Kind identifies the tool, e.g. "jmeter".
	Kind() string
	// Trigger starts the test on the engine at engineURL with cfg. Triggering
	// an already-running engine is a no-op (not an error).
	Trigger(ctx context.Context, engineURL string, cfg engine.Config) error
	// Stop stops the test on the engine at engineURL. Stopping an idle engine
	// is a no-op.
	Stop(ctx context.Context, engineURL string) error
	// Progress reports whether the engine is still running a test.
	Progress(ctx context.Context, engineURL string) (bool, error)
	// Subscribe streams metric events from the engine until ctx is cancelled or
	// the stream ends. The returned channel is closed when streaming stops.
	Subscribe(ctx context.Context, engineURL string) (<-chan engine.Metric, error)
}
