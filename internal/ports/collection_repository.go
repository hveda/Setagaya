package ports

import (
	"context"

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
)

// CollectionRepository persists Collection aggregates, their data files, and
// their execution configuration.
type CollectionRepository interface {
	CreateCollection(ctx context.Context, c execution.Execution) (int64, error)
	GetCollection(ctx context.Context, id int64) (execution.Execution, error)
	ListCollectionsByProject(ctx context.Context, projectID int64) ([]execution.Execution, error)
	DeleteCollection(ctx context.Context, id int64) error

	AddCollectionFile(ctx context.Context, executionID int64, filename string) error
	CollectionFilesFor(ctx context.Context, executionID int64) ([]string, error)
	DeleteCollectionFile(ctx context.Context, executionID int64, filename string) error

	// StoreExecutionCollection replaces the collection's execution plans with
	// plans and updates its csv_split flag, atomically.
	StoreExecutionCollection(ctx context.Context, executionID int64, csvSplit bool, plans []loadprofile.Entry) error
	// ExecutionPlansFor returns the collection's current execution plans.
	ExecutionPlansFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
}
