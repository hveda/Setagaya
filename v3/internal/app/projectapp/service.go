// Package projectapp implements the application use-cases for projects. It
// orchestrates the pure domain (internal/domain/project) over the repository
// ports, and holds no infrastructure dependencies of its own.
package projectapp

import (
	"context"
	"errors"

	"github.com/hveda/Setagaya/v3/internal/domain/collection"
	"github.com/hveda/Setagaya/v3/internal/domain/plan"
	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// Business-rule errors. Callers compare with errors.Is.
var (
	ErrProjectHasPlans       = errors.New("projectapp: cannot delete a project that has plans")
	ErrProjectHasCollections = errors.New("projectapp: cannot delete a project that has collections")
)

// Repo is the repository surface the project service needs: project CRUD plus
// the child listings used to enforce delete rules.
type Repo interface {
	ports.ProjectRepository
	ListPlansByProject(ctx context.Context, projectID int64) ([]plan.Plan, error)
	ListCollectionsByProject(ctx context.Context, projectID int64) ([]collection.Collection, error)
}

// Service provides project use-cases.
type Service struct {
	repo Repo
}

// NewService wires a Service to a project repository.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// List returns the projects owned by any of owners.
func (s *Service) List(ctx context.Context, owners []string) ([]project.Project, error) {
	return s.repo.ListProjectsByOwners(ctx, owners)
}

// Get returns a single project by ID (ports.ErrNotFound if absent).
func (s *Service) Get(ctx context.Context, id int64) (project.Project, error) {
	return s.repo.GetProject(ctx, id)
}

// Create validates the input, persists a new project, and returns it with its
// assigned ID.
func (s *Service) Create(ctx context.Context, name, owner, sid string) (project.Project, error) {
	p, err := project.New(name, owner, sid)
	if err != nil {
		return project.Project{}, err
	}
	id, err := s.repo.CreateProject(ctx, p)
	if err != nil {
		return project.Project{}, err
	}
	p.ID = id
	return p, nil
}

// Delete removes a project by ID. It refuses to delete a project that still has
// plans or collections (ports.ErrNotFound if the project is absent).
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.GetProject(ctx, id); err != nil {
		return err
	}
	plans, err := s.repo.ListPlansByProject(ctx, id)
	if err != nil {
		return err
	}
	if len(plans) > 0 {
		return ErrProjectHasPlans
	}
	collections, err := s.repo.ListCollectionsByProject(ctx, id)
	if err != nil {
		return err
	}
	if len(collections) > 0 {
		return ErrProjectHasCollections
	}
	return s.repo.DeleteProject(ctx, id)
}
