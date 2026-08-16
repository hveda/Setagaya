package ports

import (
	"context"
	"time"
)

// OrphanCompletion is a shard's Final batch that arrived with no open run to
// absorb it. It is the control plane's only reliable signal that an
// execution's engines already finished -- the scheduler cannot see it (a pod
// stays Ready forever after bzt exits), but the sidecar's last push says it
// plainly.
type OrphanCompletion struct {
	ExecutionID int64
	ScenarioID  int64
	ShardIndex  int
	// ExitCode is bzt's exit code when the Final batch carried one; nil when
	// the shard said it was done without saying how it ended.
	ExitCode *int
	// FinishedAt is when the orphaned Final was absorbed.
	FinishedAt time.Time
}

// OrphanRepository records orphaned shard completions: the "engines finished,
// nobody triggered" evidence Trigger refuses to open a corpse-run against, and
// Deploy clears when it genuinely replaces the engines.
type OrphanRepository interface {
	// RecordOrphanCompletion stores an orphaned shard completion for an
	// execution. A shard's Final is one event no matter how many times its
	// sidecar retries the push, so re-recording the same
	// execution/scenario/shard overwrites rather than accumulates.
	RecordOrphanCompletion(ctx context.Context, oc OrphanCompletion) error
	// OrphanCompletions lists an execution's orphaned completions.
	OrphanCompletions(ctx context.Context, executionID int64) ([]OrphanCompletion, error)
	// ClearOrphanCompletions drops every orphan row for an execution, called
	// when a new Deploy replaces the engines the orphans described.
	ClearOrphanCompletions(ctx context.Context, executionID int64) error
}
