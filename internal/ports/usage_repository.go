package ports

import (
	"context"
	"errors"
	"time"
)

// ErrLaunchActive is returned by StartLaunch when a collection already has an
// open (unfinished) launch.
var ErrLaunchActive = errors.New("ports: a launch is already active for this collection")

// LaunchRecord is one row of collection launch history for usage accounting.
type LaunchRecord struct {
	ExecutionID int64
	Context     string
	Owner       string
	Engines     int
	VU          int
	StartedTime time.Time
	EndTime     *time.Time
}

// UsageRepository records test launches and answers usage queries. It reuses
// the v2 collection_launch (active-launch guard) and collection_launch_history2
// (history) tables.
type UsageRepository interface {
	// StartLaunch opens a launch for a collection in this deployment context.
	// Returns ErrLaunchActive if one is already open.
	StartLaunch(ctx context.Context, executionID int64, owner string, engines, vu int) error
	// FinishLaunch stamps the open launch's end time and final VU, and clears
	// the active-launch guard. Finishing with no open launch is not an error.
	FinishLaunch(ctx context.Context, executionID int64, vu int) error
	// LaunchHistory returns finished launches whose start/end fall within
	// [from, to].
	LaunchHistory(ctx context.Context, from, to time.Time) ([]LaunchRecord, error)
}
