// Package scenarioapp implements the application use-cases for scenarios and their
// files (a JMX test file plus data files). It coordinates the scenario repository
// and the object store; the storage key convention is "scenario/{id}/{filename}".
package scenarioapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/heridotlife/honryu/internal/domain/jmx"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// Business-rule errors. Callers compare with errors.Is.
var (
	ErrScenarioInUse   = errors.New("scenarioapp: scenario is in use by an execution")
	ErrInvalidFilename = errors.New("scenarioapp: invalid filename")
)

// Repo is the repository surface the scenario service needs.
type Repo interface {
	CreateScenario(ctx context.Context, p scenario.Scenario) (int64, error)
	GetScenario(ctx context.Context, id int64) (scenario.Scenario, error)
	ListScenariosByProject(ctx context.Context, projectID int64) ([]scenario.Scenario, error)
	DeleteScenario(ctx context.Context, id int64) error
	AddScenarioFile(ctx context.Context, scenarioID int64, filename string, isTest bool) error
	ScenarioFilesFor(ctx context.Context, scenarioID int64) (ports.ScenarioFiles, error)
	DeleteScenarioFile(ctx context.Context, scenarioID int64, filename string, isTest bool) error
	ScenarioInUse(ctx context.Context, scenarioID int64) (bool, error)
}

// Service provides scenario use-cases.
type Service struct {
	repo  Repo
	store ports.ObjectStore
}

// NewService wires a Service to a scenario repository and an object store.
func NewService(repo Repo, store ports.ObjectStore) *Service {
	return &Service{repo: repo, store: store}
}

// FileRef describes a stored file and how to fetch it.
type FileRef struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
}

// Files lists a scenario's files with retrieval URLs.
type Files struct {
	TestFile *FileRef  `json:"test_file"`
	Data     []FileRef `json:"data"`
}

// Create validates input and persists a new scenario.
func (s *Service) Create(ctx context.Context, name string, projectID int64) (scenario.Scenario, error) {
	p, err := scenario.New(name, projectID)
	if err != nil {
		return scenario.Scenario{}, err
	}
	id, err := s.repo.CreateScenario(ctx, p)
	if err != nil {
		return scenario.Scenario{}, err
	}
	p.ID = id
	return p, nil
}

// Get returns a scenario by ID (ports.ErrNotFound if absent).
func (s *Service) Get(ctx context.Context, id int64) (scenario.Scenario, error) {
	return s.repo.GetScenario(ctx, id)
}

// ListByProject returns the scenarios belonging to a project.
func (s *Service) ListByProject(ctx context.Context, projectID int64) ([]scenario.Scenario, error) {
	return s.repo.ListScenariosByProject(ctx, projectID)
}

// Delete removes a scenario (and its files) unless it is used by an execution.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.GetScenario(ctx, id); err != nil {
		return err
	}
	inUse, err := s.repo.ScenarioInUse(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return ErrScenarioInUse
	}
	files, err := s.repo.ScenarioFilesFor(ctx, id)
	if err != nil {
		return err
	}
	for _, name := range allFileNames(files) {
		if delErr := s.store.Delete(ctx, scenarioKey(id, name)); delErr != nil {
			return delErr
		}
	}
	return s.repo.DeleteScenario(ctx, id)
}

// ImportResult is a completed JMX import: the scenario that now exists, and
// what the inspector found in the plan.
type ImportResult struct {
	Scenario scenario.Scenario `json:"scenario"`
	Report   jmx.Report        `json:"report"`
}

// ImportJMX creates a scenario from a Shibuya JMeter plan.
//
// The plan is not converted: Taurus runs a .jmx directly, so the scenario is
// native and pinned to JMeter. What the import adds is the inspector's report --
// the plan's own load settings that Honryu overrides, data files the pod must be
// given, and listeners that will not do what their author expects. The file is
// stored only after it has been inspected, so an unusable plan never becomes a
// scenario.
func (s *Service) ImportJMX(ctx context.Context, name string, projectID int64, filename string, content io.Reader) (ImportResult, error) {
	if err := validateFilename(filename); err != nil {
		return ImportResult{}, err
	}
	if !isJMX(filename) {
		return ImportResult{}, fmt.Errorf("%w: %q is not a .jmx file", ErrInvalidFilename, filename)
	}

	raw, err := io.ReadAll(content)
	if err != nil {
		return ImportResult{}, fmt.Errorf("scenarioapp: read plan: %w", err)
	}
	report, err := jmx.Inspect(bytes.NewReader(raw))
	if err != nil {
		return ImportResult{}, err
	}

	sc, err := scenario.NewNative(strings.TrimSpace(name), projectID, taurus.ExecutorJMeter)
	if err != nil {
		return ImportResult{}, err
	}
	id, err := s.repo.CreateScenario(ctx, sc)
	if err != nil {
		return ImportResult{}, err
	}
	sc.ID = id

	if err := s.UploadFile(ctx, id, filename, bytes.NewReader(raw)); err != nil {
		// Roll back, so a failed import leaves no scenario behind.
		_ = s.repo.DeleteScenario(ctx, id)
		return ImportResult{}, err
	}
	return ImportResult{Scenario: sc, Report: report}, nil
}

// Files returns the scenario's files with URLs.
func (s *Service) Files(ctx context.Context, scenarioID int64) (Files, error) {
	pf, err := s.repo.ScenarioFilesFor(ctx, scenarioID)
	if err != nil {
		return Files{}, err
	}
	out := Files{Data: make([]FileRef, 0, len(pf.Data))}
	if pf.TestFile != "" {
		out.TestFile = &FileRef{Filename: pf.TestFile, URL: s.store.URL(scenarioKey(scenarioID, pf.TestFile))}
	}
	for _, name := range pf.Data {
		out.Data = append(out.Data, FileRef{Filename: name, URL: s.store.URL(scenarioKey(scenarioID, name))})
	}
	return out, nil
}

// UploadFile records and stores a scenario file. A ".jmx" file is stored as the
// scenario's single test file; anything else is a data file. Returns
// ports.ErrFileExists if the file is already present.
func (s *Service) UploadFile(ctx context.Context, scenarioID int64, filename string, content io.Reader) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	if _, err := s.repo.GetScenario(ctx, scenarioID); err != nil {
		return err
	}
	isTest := isJMX(filename)
	if err := s.repo.AddScenarioFile(ctx, scenarioID, filename, isTest); err != nil {
		return err
	}
	if err := s.store.Upload(ctx, scenarioKey(scenarioID, filename), content); err != nil {
		// Roll back the record so it does not dangle without an object.
		_ = s.repo.DeleteScenarioFile(ctx, scenarioID, filename, isTest)
		return err
	}
	return nil
}

// DownloadFile returns the bytes of a scenario file (ports.ErrObjectNotFound if
// absent).
func (s *Service) DownloadFile(ctx context.Context, scenarioID int64, filename string) ([]byte, error) {
	if err := validateFilename(filename); err != nil {
		return nil, err
	}
	return s.store.Download(ctx, scenarioKey(scenarioID, filename))
}

// DeleteFile removes a scenario file record and its stored object.
func (s *Service) DeleteFile(ctx context.Context, scenarioID int64, filename string) error {
	if err := validateFilename(filename); err != nil {
		return err
	}
	isTest := isJMX(filename)
	if err := s.repo.DeleteScenarioFile(ctx, scenarioID, filename, isTest); err != nil {
		return err
	}
	return s.store.Delete(ctx, scenarioKey(scenarioID, filename))
}

func scenarioKey(scenarioID int64, filename string) string {
	return fmt.Sprintf("scenario/%d/%s", scenarioID, filename)
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

func allFileNames(pf ports.ScenarioFiles) []string {
	names := append([]string(nil), pf.Data...)
	if pf.TestFile != "" {
		names = append(names, pf.TestFile)
	}
	return names
}
