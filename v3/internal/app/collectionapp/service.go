// Package collectionapp implements the application use-cases for collections:
// CRUD, data files, and the execution configuration (which plans run, with how
// many engines). Storage keys follow "collection/{id}/{filename}".
package collectionapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/domain/execution"
	"github.com/heridotlife/Setagaya/v3/internal/domain/plan"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Business-rule errors. Callers compare with errors.Is.
var (
	ErrInvalidFilename    = errors.New("collectionapp: invalid filename")
	ErrCollectionMismatch = errors.New("collectionapp: config collection id does not match target")
	ErrPlanNotInProject   = errors.New("collectionapp: plan does not belong to the collection's project")
	ErrEngineLimit        = errors.New("collectionapp: requested engines exceed the cluster limit")
)

// Repo is the repository surface the collection service needs, including
// GetPlan to validate execution config against real plans.
type Repo interface {
	CreateCollection(ctx context.Context, c collection.Collection) (int64, error)
	GetCollection(ctx context.Context, id int64) (collection.Collection, error)
	ListCollectionsByProject(ctx context.Context, projectID int64) ([]collection.Collection, error)
	DeleteCollection(ctx context.Context, id int64) error
	AddCollectionFile(ctx context.Context, collectionID int64, filename string) error
	CollectionFilesFor(ctx context.Context, collectionID int64) ([]string, error)
	DeleteCollectionFile(ctx context.Context, collectionID int64, filename string) error
	StoreExecutionCollection(ctx context.Context, collectionID int64, csvSplit bool, plans []execution.ExecutionPlan) error
	ExecutionPlansFor(ctx context.Context, collectionID int64) ([]execution.ExecutionPlan, error)
	GetPlan(ctx context.Context, id int64) (plan.Plan, error)
}

// Service provides collection use-cases.
type Service struct {
	repo       Repo
	store      ports.ObjectStore
	maxEngines int
}

// NewService wires a Service. maxEngines caps the total engines a collection's
// execution config may request.
func NewService(repo Repo, store ports.ObjectStore, maxEngines int) *Service {
	return &Service{repo: repo, store: store, maxEngines: maxEngines}
}

// FileRef describes a stored file and how to fetch it.
type FileRef struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// Create validates input and persists a new collection.
func (s *Service) Create(ctx context.Context, name string, projectID int64) (collection.Collection, error) {
	c, err := collection.New(name, projectID)
	if err != nil {
		return collection.Collection{}, err
	}
	id, err := s.repo.CreateCollection(ctx, c)
	if err != nil {
		return collection.Collection{}, err
	}
	c.ID = id
	return c, nil
}

// Get returns a collection by ID (ports.ErrNotFound if absent).
func (s *Service) Get(ctx context.Context, id int64) (collection.Collection, error) {
	return s.repo.GetCollection(ctx, id)
}

// ListByProject returns the collections of a project.
func (s *Service) ListByProject(ctx context.Context, projectID int64) ([]collection.Collection, error) {
	return s.repo.ListCollectionsByProject(ctx, projectID)
}

// Delete removes a collection and its data files.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.GetCollection(ctx, id); err != nil {
		return err
	}
	files, err := s.repo.CollectionFilesFor(ctx, id)
	if err != nil {
		return err
	}
	for _, name := range files {
		if delErr := s.store.Delete(ctx, collectionKey(id, name)); delErr != nil {
			return delErr
		}
	}
	return s.repo.DeleteCollection(ctx, id)
}

// Files lists a collection's data files with retrieval URLs.
func (s *Service) Files(ctx context.Context, collectionID int64) ([]FileRef, error) {
	names, err := s.repo.CollectionFilesFor(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	out := make([]FileRef, 0, len(names))
	for _, name := range names {
		out = append(out, FileRef{Filename: name, URL: s.store.URL(collectionKey(collectionID, name))})
	}
	return out, nil
}

// UploadFile records and stores a collection data file. Returns
// ports.ErrFileExists if it is already present.
func (s *Service) UploadFile(ctx context.Context, collectionID int64, filename string, content io.Reader) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	if _, err := s.repo.GetCollection(ctx, collectionID); err != nil {
		return err
	}
	if err := s.repo.AddCollectionFile(ctx, collectionID, filename); err != nil {
		return err
	}
	if err := s.store.Upload(ctx, collectionKey(collectionID, filename), content); err != nil {
		_ = s.repo.DeleteCollectionFile(ctx, collectionID, filename)
		return err
	}
	return nil
}

// DownloadFile returns the bytes of a collection data file.
func (s *Service) DownloadFile(ctx context.Context, collectionID int64, filename string) ([]byte, error) {
	if err := validateFilename(filename); err != nil {
		return nil, err
	}
	return s.store.Download(ctx, collectionKey(collectionID, filename))
}

// DeleteFile removes a collection data file record and its stored object.
func (s *Service) DeleteFile(ctx context.Context, collectionID int64, filename string) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	if err := s.repo.DeleteCollectionFile(ctx, collectionID, filename); err != nil {
		return err
	}
	return s.store.Delete(ctx, collectionKey(collectionID, filename))
}

// StoreConfig validates and persists the execution configuration for a
// collection: every plan must exist and belong to the collection's project, and
// the total engines must not exceed the configured limit.
func (s *Service) StoreConfig(ctx context.Context, collectionID int64, ec execution.ExecutionCollection) error {
	coll, err := s.repo.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	if ec.CollectionID != collectionID {
		return ErrCollectionMismatch
	}
	if err := ec.Validate(); err != nil {
		return err
	}
	for _, ep := range ec.Tests {
		p, planErr := s.repo.GetPlan(ctx, ep.PlanID)
		if planErr != nil {
			return planErr
		}
		if p.ProjectID != coll.ProjectID {
			return ErrPlanNotInProject
		}
	}
	if total := ec.TotalEngines(); total > s.maxEngines {
		return fmt.Errorf("%w: requested %d, limit %d", ErrEngineLimit, total, s.maxEngines)
	}
	return s.repo.StoreExecutionCollection(ctx, collectionID, ec.CSVSplit, ec.Tests)
}

// GetConfig returns the collection's current execution configuration wrapped
// for serialization.
func (s *Service) GetConfig(ctx context.Context, collectionID int64) (execution.Wrapper, error) {
	coll, err := s.repo.GetCollection(ctx, collectionID)
	if err != nil {
		return execution.Wrapper{}, err
	}
	plans, err := s.repo.ExecutionPlansFor(ctx, collectionID)
	if err != nil {
		return execution.Wrapper{}, err
	}
	return execution.Wrapper{Content: execution.ExecutionCollection{
		Name:         coll.Name,
		ProjectID:    coll.ProjectID,
		CollectionID: collectionID,
		Tests:        plans,
		CSVSplit:     coll.CSVSplit,
	}}, nil
}

func collectionKey(collectionID int64, filename string) string {
	return fmt.Sprintf("collection/%d/%s", collectionID, filename)
}

func validateFilename(filename string) error {
	if filename == "" || strings.ContainsAny(filename, "/\\") || filename == "." || filename == ".." {
		return ErrInvalidFilename
	}
	return nil
}
