package ports

import (
	"context"
	"errors"
	"time"
)

// ErrRunActive is returned by StartRun when a collection already has an active
// run. ErrNoActiveRun is returned when an operation needs one and none exists.
var (
	ErrRunActive   = errors.New("ports: a run is already active for this collection")
	ErrNoActiveRun = errors.New("ports: no active run for this collection")
)

// RunRecord is a row of collection run history.
type RunRecord struct {
	RunID       int64
	ExecutionID int64
	StartedTime time.Time
	EndTime     *time.Time
}

// RunningPlan marks a plan currently executing within a collection.
type RunningPlan struct {
	ExecutionID int64
	PlanID      int64
	StartedTime time.Time
}

// RunRepository persists the lifecycle state of runs: the single active run per
// collection, its history, and which plans are currently executing. It reuses
// the v2 collection_run, collection_run_history, and running_plan tables.
type RunRepository interface {
	// StartRun creates the active run for a collection and opens a history row,
	// returning the new run id. Returns ErrRunActive if one already exists.
	StartRun(ctx context.Context, executionID int64) (int64, error)
	// CurrentRun returns the active run id for a collection; ok is false when
	// there is none.
	CurrentRun(ctx context.Context, executionID int64) (runID int64, ok bool, err error)
	// StopRun clears the active run and stamps end_time on its history row.
	// Stopping a collection with no active run is not an error.
	StopRun(ctx context.Context, executionID int64) error
	// MarkPlanRunning records that a plan is executing (idempotent).
	MarkPlanRunning(ctx context.Context, executionID, planID int64) error
	// ClearPlanRunning removes a plan's running marker (idempotent).
	ClearPlanRunning(ctx context.Context, executionID, planID int64) error
	// RunningPlans lists every running plan in this deployment context, used to
	// resume tracking after a controller restart.
	RunningPlans(ctx context.Context) ([]RunningPlan, error)
	// RunningPlansByCollection lists running plans for one collection.
	RunningPlansByCollection(ctx context.Context, executionID int64) ([]RunningPlan, error)
}
