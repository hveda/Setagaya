package ports

import (
	"context"

	"github.com/heridotlife/Setagaya/internal/domain/collection"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
)

// CollectionRepository persists Collection aggregates, their data files, and
// their execution configuration.
type CollectionRepository interface {
	CreateCollection(ctx context.Context, c collection.Collection) (int64, error)
	GetCollection(ctx context.Context, id int64) (collection.Collection, error)
	ListCollectionsByProject(ctx context.Context, projectID int64) ([]collection.Collection, error)
	DeleteCollection(ctx context.Context, id int64) error

	AddCollectionFile(ctx context.Context, collectionID int64, filename string) error
	CollectionFilesFor(ctx context.Context, collectionID int64) ([]string, error)
	DeleteCollectionFile(ctx context.Context, collectionID int64, filename string) error

	// StoreExecutionCollection replaces the collection's execution plans with
	// plans and updates its csv_split flag, atomically.
	StoreExecutionCollection(ctx context.Context, collectionID int64, csvSplit bool, plans []loadprofile.Entry) error
	// ExecutionPlansFor returns the collection's current execution plans.
	ExecutionPlansFor(ctx context.Context, collectionID int64) ([]loadprofile.Entry, error)
}
