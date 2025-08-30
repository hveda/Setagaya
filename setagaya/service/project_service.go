package service

import (
	"errors"
	"fmt"

	"github.com/hveda/setagaya/setagaya/auth"
	"github.com/hveda/setagaya/setagaya/model"
)

// ProjectService handles project-related business logic
type ProjectService interface {
	CreateProject(name, owner, info string) (int64, error)
	GetProject(id int64) (*model.Project, error)
	GetProjectsByOwner(owner string) ([]*model.Project, error)
	UpdateProject(id int64, name, info string, account *auth.Account) error
	DeleteProject(id int64, account *auth.Account) error
	ValidateProjectAccess(id int64, account *auth.Account) error
}

// projectService implements ProjectService
type projectService struct {
	projectRepo ProjectRepository
}

// ProjectRepository interface for data access
type ProjectRepository interface {
	Create(name, owner, info string) (int64, error)
	GetByID(id int64) (*model.Project, error)
	GetByOwner(owner string) ([]*model.Project, error)
	Update(id int64, name, info string) error
	Delete(id int64) error
	CheckOwnership(id int64, owner string) (bool, error)
}

// NewProjectService creates a new project service
func NewProjectService(repo ProjectRepository) ProjectService {
	return &projectService{
		projectRepo: repo,
	}
}

// CreateProject creates a new project with validation
func (s *projectService) CreateProject(name, owner, info string) (int64, error) {
	// Validate inputs
	if err := validateProjectName(name); err != nil {
		return 0, fmt.Errorf("invalid project name: %w", err)
	}
	
	if owner == "" {
		return 0, errors.New("owner cannot be empty")
	}
	
	// Create project through repository
	projectID, err := s.projectRepo.Create(name, owner, info)
	if err != nil {
		return 0, fmt.Errorf("failed to create project: %w", err)
	}
	
	return projectID, nil
}

// GetProject retrieves a project by ID
func (s *projectService) GetProject(id int64) (*model.Project, error) {
	if id <= 0 {
		return nil, errors.New("invalid project ID")
	}
	
	project, err := s.projectRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	
	return project, nil
}

// GetProjectsByOwner retrieves all projects for an owner
func (s *projectService) GetProjectsByOwner(owner string) ([]*model.Project, error) {
	if owner == "" {
		return nil, errors.New("owner cannot be empty")
	}
	
	projects, err := s.projectRepo.GetByOwner(owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects for owner: %w", err)
	}
	
	return projects, nil
}

// UpdateProject updates a project with authorization checks
func (s *projectService) UpdateProject(id int64, name, info string, account *auth.Account) error {
	// Validate access
	if err := s.ValidateProjectAccess(id, account); err != nil {
		return err
	}
	
	// Validate inputs
	if err := validateProjectName(name); err != nil {
		return fmt.Errorf("invalid project name: %w", err)
	}
	
	// Update through repository
	if err := s.projectRepo.Update(id, name, info); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	
	return nil
}

// DeleteProject deletes a project with authorization checks
func (s *projectService) DeleteProject(id int64, account *auth.Account) error {
	// Validate access
	if err := s.ValidateProjectAccess(id, account); err != nil {
		return err
	}
	
	// Delete through repository
	if err := s.projectRepo.Delete(id); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	
	return nil
}

// ValidateProjectAccess checks if user has access to project
func (s *projectService) ValidateProjectAccess(id int64, account *auth.Account) error {
	if account == nil {
		return errors.New("account is required")
	}
	
	// Admin users have access to all projects
	if account.IsAdmin() {
		return nil
	}
	
	// Check ownership
	hasAccess, err := s.projectRepo.CheckOwnership(id, account.Name)
	if err != nil {
		return fmt.Errorf("failed to check project ownership: %w", err)
	}
	
	if !hasAccess {
		return errors.New("insufficient permissions to access project")
	}
	
	return nil
}

// Helper functions

func validateProjectName(name string) error {
	if name == "" {
		return errors.New("project name cannot be empty")
	}
	
	if len(name) > 255 {
		return errors.New("project name too long")
	}
	
	// Additional validation can be added here
	return nil
}