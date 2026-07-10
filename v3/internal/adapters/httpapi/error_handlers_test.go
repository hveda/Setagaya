package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hveda/Setagaya/v3/internal/adapters/httpapi"
	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// boomRepo fails every read, to drive the handlers' 500 branches.
type boomRepo struct{}

func (boomRepo) CreateProject(context.Context, project.Project) (int64, error) {
	return 0, errors.New("boom")
}
func (boomRepo) GetProject(context.Context, int64) (project.Project, error) {
	return project.Project{}, errors.New("boom")
}
func (boomRepo) ListProjectsByOwners(context.Context, []string) ([]project.Project, error) {
	return nil, errors.New("boom")
}
func (boomRepo) DeleteProject(context.Context, int64) error { return errors.New("boom") }

var _ ports.ProjectRepository = boomRepo{}

func boomRouter() http.Handler {
	return httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(boomRepo{}),
		DefaultOwners: []string{"setagaya"},
	})
}

func TestListProjects_RepoError_500(t *testing.T) {
	t.Parallel()
	rec := do(t, boomRouter(), http.MethodGet, "/api/projects")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestGetProject_RepoError_500(t *testing.T) {
	t.Parallel()
	rec := do(t, boomRouter(), http.MethodGet, "/api/projects/5")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
