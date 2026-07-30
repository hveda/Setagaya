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

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Business-rule errors. Callers compare with errors.Is.
var (
	ErrInvalidFilename    = errors.New("collectionapp: invalid filename")
	ErrCollectionMismatch = errors.New("collectionapp: config collection id does not match target")
	ErrPlanNotInProject   = errors.New("collectionapp: plan does not belong to the collection's project")
	ErrEngineLimit        = errors.New("collectionapp: requested engines exceed the cluster limit")
)

// Repo is the repository surface the collection service needs, including
// GetScenario to validate execution config against real plans.
type Repo interface {
	CreateExecution(ctx context.Context, c execution.Execution) (int64, error)
	GetExecution(ctx context.Context, id int64) (execution.Execution, error)
	ListExecutionsByProject(ctx context.Context, projectID int64) ([]execution.Execution, error)
	DeleteExecution(ctx context.Context, id int64) error
	AddExecutionFile(ctx context.Context, executionID int64, filename string) error
	ExecutionFilesFor(ctx context.Context, executionID int64) ([]string, error)
	DeleteExecutionFile(ctx context.Context, executionID int64, filename string) error
	StoreLoadProfile(ctx context.Context, executionID int64, csvSplit bool, plans []loadprofile.Entry) error
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
	GetScenario(ctx context.Context, id int64) (scenario.Scenario, error)
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
func (s *Service) Create(ctx context.Context, name string, projectID int64) (execution.Execution, error) {
	c, err := execution.New(name, projectID)
	if err != nil {
		return execution.Execution{}, err
	}
	id, err := s.repo.CreateExecution(ctx, c)
	if err != nil {
		return execution.Execution{}, err
	}
	c.ID = id
	return c, nil
}

// Get returns a collection by ID (ports.ErrNotFound if absent).
func (s *Service) Get(ctx context.Context, id int64) (execution.Execution, error) {
	return s.repo.GetExecution(ctx, id)
}

// ListByProject returns the collections of a project.
func (s *Service) ListByProject(ctx context.Context, projectID int64) ([]execution.Execution, error) {
	return s.repo.ListExecutionsByProject(ctx, projectID)
}

// Delete removes a collection and its data files.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.GetExecution(ctx, id); err != nil {
		return err
	}
	files, err := s.repo.ExecutionFilesFor(ctx, id)
	if err != nil {
		return err
	}
	for _, name := range files {
		if delErr := s.store.Delete(ctx, collectionKey(id, name)); delErr != nil {
			return delErr
		}
	}
	return s.repo.DeleteExecution(ctx, id)
}

// Files lists a collection's data files with retrieval URLs.
func (s *Service) Files(ctx context.Context, executionID int64) ([]FileRef, error) {
	names, err := s.repo.ExecutionFilesFor(ctx, executionID)
	if err != nil {
		return nil, err
	}
	out := make([]FileRef, 0, len(names))
	for _, name := range names {
		out = append(out, FileRef{Filename: name, URL: s.store.URL(collectionKey(executionID, name))})
	}
	return out, nil
}

// UploadFile records and stores a collection data file. Returns
// ports.ErrFileExists if it is already present.
func (s *Service) UploadFile(ctx context.Context, executionID int64, filename string, content io.Reader) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	if _, err := s.repo.GetExecution(ctx, executionID); err != nil {
		return err
	}
	if err := s.repo.AddExecutionFile(ctx, executionID, filename); err != nil {
		return err
	}
	if err := s.store.Upload(ctx, collectionKey(executionID, filename), content); err != nil {
		_ = s.repo.DeleteExecutionFile(ctx, executionID, filename)
		return err
	}
	return nil
}

// DownloadFile returns the bytes of a collection data file.
func (s *Service) DownloadFile(ctx context.Context, executionID int64, filename string) ([]byte, error) {
	if err := validateFilename(filename); err != nil {
		return nil, err
	}
	return s.store.Download(ctx, collectionKey(executionID, filename))
}

// DeleteFile removes a collection data file record and its stored object.
func (s *Service) DeleteFile(ctx context.Context, executionID int64, filename string) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	if err := s.repo.DeleteExecutionFile(ctx, executionID, filename); err != nil {
		return err
	}
	return s.store.Delete(ctx, collectionKey(executionID, filename))
}

// StoreConfig validates and persists the execution configuration for a
// collection: every plan must exist and belong to the collection's project, and
// the total engines must not exceed the configured limit.
func (s *Service) StoreConfig(ctx context.Context, executionID int64, ec loadprofile.Profile) error {
	coll, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}
	if ec.ExecutionID != executionID {
		return ErrCollectionMismatch
	}
	if err := ec.Validate(); err != nil {
		return err
	}
	for _, ep := range ec.Tests {
		p, planErr := s.repo.GetScenario(ctx, ep.ScenarioID)
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
	return s.repo.StoreLoadProfile(ctx, executionID, ec.CSVSplit, ec.Tests)
}

// GetConfig returns the collection's current execution configuration wrapped
// for serialization.
func (s *Service) GetConfig(ctx context.Context, executionID int64) (loadprofile.Wrapper, error) {
	coll, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return loadprofile.Wrapper{}, err
	}
	plans, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return loadprofile.Wrapper{}, err
	}
	return loadprofile.Wrapper{Content: loadprofile.Profile{
		Name:        coll.Name,
		ProjectID:   coll.ProjectID,
		ExecutionID: executionID,
		Tests:       plans,
		CSVSplit:    coll.CSVSplit,
	}}, nil
}

func collectionKey(executionID int64, filename string) string {
	return fmt.Sprintf("collection/%d/%s", executionID, filename)
}

func validateFilename(filename string) error {
	if filename == "" || strings.ContainsAny(filename, "/\\") || filename == "." || filename == ".." {
		return ErrInvalidFilename
	}
	return nil
}
