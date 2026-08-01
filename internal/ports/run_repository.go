package ports

import (
	"context"
	"errors"
	"time"
)

// ErrRunActive is returned by StartRun when an execution already has an active
// run. ErrNoActiveRun is returned when an operation needs one and none exists.
var (
	ErrRunActive   = errors.New("ports: a run is already active for this execution")
	ErrNoActiveRun = errors.New("ports: no active run for this execution")
)

// RunRecord is a row of execution run history.
type RunRecord struct {
	RunID       int64
	ExecutionID int64
	StartedTime time.Time
	EndTime     *time.Time
}

// RunningScenario marks a scenario currently executing within an execution.
type RunningScenario struct {
	ExecutionID int64
	ScenarioID  int64
	StartedTime time.Time
}

// RunRepository persists the lifecycle state of runs: the single active run per
// execution, its history, and which scenarios are currently executing. It reuses
// the execution_run, execution_run_history, and running_scenario tables.
type RunRepository interface {
	// StartRun creates the active run for an execution and opens a history row,
	// returning the new run id. Returns ErrRunActive if one already exists.
	StartRun(ctx context.Context, executionID int64) (int64, error)
	// CurrentRun returns the active run id for an execution; ok is false when
	// there is none.
	CurrentRun(ctx context.Context, executionID int64) (runID int64, ok bool, err error)
	// StopRun clears the active run and stamps end_time on its history row.
	// Stopping an execution with no active run is not an error.
	StopRun(ctx context.Context, executionID int64) error
	// RunHistory returns a run's history record, or ErrNotFound. It is how a
	// report learns when its run started, since nothing else keeps that once the
	// run itself has been superseded or stopped.
	RunHistory(ctx context.Context, runID int64) (RunRecord, error)
	// MarkScenarioRunning records that a scenario is executing (idempotent).
	MarkScenarioRunning(ctx context.Context, executionID, scenarioID int64) error
	// ClearScenarioRunning removes a scenario's running marker (idempotent).
	ClearScenarioRunning(ctx context.Context, executionID, scenarioID int64) error
	// RunningScenarios lists every running scenario in this deployment context, used to
	// resume tracking after a controller restart.
	RunningScenarios(ctx context.Context) ([]RunningScenario, error)
	// RunningScenariosByExecution lists running scenarios for one execution.
	RunningScenariosByExecution(ctx context.Context, executionID int64) ([]RunningScenario, error)
}
