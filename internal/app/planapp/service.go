// Package planapp implements the application use-cases for plans and their
// files (a JMX test file plus data files). It coordinates the plan repository
// and the object store; the storage key convention is "plan/{id}/{filename}".
package planapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Business-rule errors. Callers compare with errors.Is.
var (
	ErrPlanInUse       = errors.New("planapp: plan is in use by a collection")
	ErrInvalidFilename = errors.New("planapp: invalid filename")
)

// Repo is the repository surface the plan service needs.
type Repo interface {
	CreatePlan(ctx context.Context, p scenario.Scenario) (int64, error)
	GetPlan(ctx context.Context, id int64) (scenario.Scenario, error)
	ListPlansByProject(ctx context.Context, projectID int64) ([]scenario.Scenario, error)
	DeletePlan(ctx context.Context, id int64) error
	AddPlanFile(ctx context.Context, planID int64, filename string, isTest bool) error
	PlanFilesFor(ctx context.Context, planID int64) (ports.PlanFiles, error)
	DeletePlanFile(ctx context.Context, planID int64, filename string, isTest bool) error
	PlanInUse(ctx context.Context, planID int64) (bool, error)
}

// Service provides plan use-cases.
type Service struct {
	repo  Repo
	store ports.ObjectStore
}

// NewService wires a Service to a plan repository and an object store.
func NewService(repo Repo, store ports.ObjectStore) *Service {
	return &Service{repo: repo, store: store}
}

// FileRef describes a stored file and how to fetch it.
type FileRef struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// Files lists a plan's files with retrieval URLs.
type Files struct {
	TestFile *FileRef  `json:"test_file"`
	Data     []FileRef `json:"data"`
}

// Create validates input and persists a new plan.
func (s *Service) Create(ctx context.Context, name string, projectID int64) (scenario.Scenario, error) {
	p, err := scenario.New(name, projectID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	id, err := s.repo.CreatePlan(ctx, p)
	if err != nil {
		return scenario.Scenario{}, err
	}
	p.ID = id
	return p, nil
}

// Get returns a plan by ID (ports.ErrNotFound if absent).
func (s *Service) Get(ctx context.Context, id int64) (scenario.Scenario, error) {
	return s.repo.GetPlan(ctx, id)
}

// ListByProject returns the plans belonging to a project.
func (s *Service) ListByProject(ctx context.Context, projectID int64) ([]scenario.Scenario, error) {
	return s.repo.ListPlansByProject(ctx, projectID)
}

// Delete removes a plan (and its files) unless it is used by a collection.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.GetPlan(ctx, id); err != nil {
		return err
	}
	inUse, err := s.repo.PlanInUse(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return ErrPlanInUse
	}
	files, err := s.repo.PlanFilesFor(ctx, id)
	if err != nil {
		return err
	}
	for _, name := range allFileNames(files) {
		if delErr := s.store.Delete(ctx, planKey(id, name)); delErr != nil {
			return delErr
		}
	}
	return s.repo.DeletePlan(ctx, id)
}

// Files returns the plan's files with URLs.
func (s *Service) Files(ctx context.Context, planID int64) (Files, error) {
	pf, err := s.repo.PlanFilesFor(ctx, planID)
	if err != nil {
		return Files{}, err
	}
	out := Files{Data: make([]FileRef, 0, len(pf.Data))}
	if pf.TestFile != "" {
		out.TestFile = &FileRef{Filename: pf.TestFile, URL: s.store.URL(planKey(planID, pf.TestFile))}
	}
	for _, name := range pf.Data {
		out.Data = append(out.Data, FileRef{Filename: name, URL: s.store.URL(planKey(planID, name))})
	}
	return out, nil
}

// UploadFile records and stores a plan file. A ".jmx" file is stored as the
// plan's single test file; anything else is a data file. Returns
// ports.ErrFileExists if the file is already present.
func (s *Service) UploadFile(ctx context.Context, planID int64, filename string, content io.Reader) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	if _, err := s.repo.GetPlan(ctx, planID); err != nil {
		return err
	}
	isTest := isJMX(filename)
	if err := s.repo.AddPlanFile(ctx, planID, filename, isTest); err != nil {
		return err
	}
	if err := s.store.Upload(ctx, planKey(planID, filename), content); err != nil {
		// Roll back the record so it does not dangle without an object.
		_ = s.repo.DeletePlanFile(ctx, planID, filename, isTest)
		return err
	}
	return nil
}

// DownloadFile returns the bytes of a plan file (ports.ErrObjectNotFound if
// absent).
func (s *Service) DownloadFile(ctx context.Context, planID int64, filename string) ([]byte, error) {
	if err := validateFilename(filename); err != nil {
		return nil, err
	}
	return s.store.Download(ctx, planKey(planID, filename))
}

// DeleteFile removes a plan file record and its stored object.
func (s *Service) DeleteFile(ctx context.Context, planID int64, filename string) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	isTest := isJMX(filename)
	if err := s.repo.DeletePlanFile(ctx, planID, filename, isTest); err != nil {
		return err
	}
	return s.store.Delete(ctx, planKey(planID, filename))
}

func planKey(planID int64, filename string) string {
	return fmt.Sprintf("plan/%d/%s", planID, filename)
}

func isJMX(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".jmx")
}

func validateFilename(filename string) error {
	if filename == "" || strings.ContainsAny(filename, "/\\") || filename == "." || filename == ".." {
		return ErrInvalidFilename
	}
	return nil
}

func allFileNames(pf ports.PlanFiles) []string {
	names := append([]string(nil), pf.Data...)
	if pf.TestFile != "" {
		names = append(names, pf.TestFile)
	}
	return names
}
