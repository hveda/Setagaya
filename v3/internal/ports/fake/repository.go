// Package fake provides in-memory implementations of the repository ports for
// fast, hermetic unit tests of the application layer. They are safe for
// concurrent use and mimic the observable behaviour of the real adapters as
// pinned by internal/ports/repositorytest.
package fake

import (
	"context"
	"sync"
	"time"

	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// ProjectRepository is an in-memory ports.ProjectRepository.
type ProjectRepository struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]project.Project
	now    func() time.Time
}

// NewProjectRepository returns an empty in-memory ProjectRepository.
func NewProjectRepository() *ProjectRepository {
	return &ProjectRepository{
		items: make(map[int64]project.Project),
		now:   time.Now,
	}
}

var _ ports.ProjectRepository = (*ProjectRepository)(nil)

// CreateProject stores a copy of p with a freshly assigned ID and timestamp.
func (r *ProjectRepository) CreateProject(_ context.Context, p project.Project) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	p.ID = r.nextID
	if p.CreatedTime.IsZero() {
		p.CreatedTime = r.now()
	}
	r.items[p.ID] = p
	return p.ID, nil
}

// GetProject returns the project with id, or ports.ErrNotFound.
func (r *ProjectRepository) GetProject(_ context.Context, id int64) (project.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.items[id]
	if !ok {
		return project.Project{}, ports.ErrNotFound
	}
	return p, nil
}

// ListProjectsByOwners returns projects owned by any of owners.
func (r *ProjectRepository) ListProjectsByOwners(_ context.Context, owners []string) ([]project.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	want := make(map[string]struct{}, len(owners))
	for _, o := range owners {
		want[o] = struct{}{}
	}
	out := []project.Project{}
	for _, p := range r.items {
		if _, ok := want[p.Owner]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// DeleteProject removes id, or returns ports.ErrNotFound.
func (r *ProjectRepository) DeleteProject(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return ports.ErrNotFound
	}
	delete(r.items, id)
	return nil
}
