package projectapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// errRepo is a ProjectRepository whose write path always fails, used to verify
// the service surfaces repository errors from Create.
type errRepo struct {
	err error
}

func (r errRepo) CreateProject(context.Context, project.Project) (int64, error) { return 0, r.err }
func (r errRepo) GetProject(context.Context, int64) (project.Project, error) {
	return project.Project{}, r.err
}
func (r errRepo) ListProjectsByOwners(context.Context, []string) ([]project.Project, error) {
	return nil, r.err
}
func (r errRepo) DeleteProject(context.Context, int64) error { return r.err }

var _ ports.ProjectRepository = errRepo{}

func TestService_Create_PropagatesRepoError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("db exploded")
	svc := projectapp.NewService(errRepo{err: sentinel})

	_, err := svc.Create(context.Background(), "web-api", "team-a", "1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Create err = %v, want %v", err, sentinel)
	}
}
