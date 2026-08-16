// Package ports declares the interfaces (hexagonal "ports") that the
// application layer depends on. Concrete adapters (MySQL, Kubernetes, object
// storage, ...) live under internal/adapters and are injected at wiring time,
// so the application and domain never import infrastructure directly.
package ports

import (
	"context"
	"errors"

	"github.com/heridotlife/honryu/internal/domain/project"
)

// ErrNotFound is returned by repositories when a requested entity does not
// exist. Callers compare with errors.Is.
var ErrNotFound = errors.New("ports: not found")

// ErrFileExists is returned when adding a file that already exists for the
// owning scenario or execution. Callers compare with errors.Is.
var ErrFileExists = errors.New("ports: file already exists")

// ProjectRepository persists and retrieves Project aggregates.
type ProjectRepository interface {
	// CreateProject stores p and returns its newly-assigned ID.
	CreateProject(ctx context.Context, p project.Project) (int64, error)
	// GetProject returns the project with the given ID, or ErrNotFound.
	GetProject(ctx context.Context, id int64) (project.Project, error)
	// ListProjectsByOwners returns all projects owned by any of owners.
	// An empty owners slice returns an empty result and no error.
	ListProjectsByOwners(ctx context.Context, owners []string) ([]project.Project, error)
	// ListAllProjects returns every project (service-provider admin view).
	ListAllProjects(ctx context.Context) ([]project.Project, error)
	// ListProjectsByTenants returns all projects belonging to any of tenantIDs.
	// An empty tenantIDs slice returns an empty result and no error.
	ListProjectsByTenants(ctx context.Context, tenantIDs []int64) ([]project.Project, error)
	// DeleteProject removes the project with the given ID, or ErrNotFound.
	DeleteProject(ctx context.Context, id int64) error
}
