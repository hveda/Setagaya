package ports

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
)

// ExecutionRepository persists Execution aggregates, their data files, and
// their execution configuration.
type ExecutionRepository interface {
	CreateExecution(ctx context.Context, c execution.Execution) (int64, error)
	GetExecution(ctx context.Context, id int64) (execution.Execution, error)
	ListExecutionsByProject(ctx context.Context, projectID int64) ([]execution.Execution, error)
	DeleteExecution(ctx context.Context, id int64) error

	AddExecutionFile(ctx context.Context, executionID int64, filename string) error
	ExecutionFilesFor(ctx context.Context, executionID int64) ([]string, error)
	DeleteExecutionFile(ctx context.Context, executionID int64, filename string) error

	// StoreLoadProfile replaces the execution's load profile entries with
	// entries and updates its csv_split flag, atomically.
	StoreLoadProfile(ctx context.Context, executionID int64, csvSplit bool, entries []loadprofile.Entry) error
	// LoadProfileFor returns the execution's current load profile entries.
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
}
