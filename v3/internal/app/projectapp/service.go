// Package projectapp implements the application use-cases for projects. It
// orchestrates the pure domain (internal/domain/project) over the repository
// port, and holds no infrastructure dependencies of its own.
package projectapp

import (
	"context"

	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// Service provides project use-cases.
type Service struct {
	repo ports.ProjectRepository
}

// NewService wires a Service to a project repository.
func NewService(repo ports.ProjectRepository) *Service {
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

// Delete removes a project by ID (ports.ErrNotFound if absent).
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.DeleteProject(ctx, id)
}
