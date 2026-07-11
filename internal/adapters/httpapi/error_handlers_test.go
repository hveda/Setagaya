package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

// boomRepo embeds a real fake store but forces the project reads to fail,
// driving the handlers' 500 branches.
type boomRepo struct {
	*fake.Store
}

func (boomRepo) GetProject(context.Context, int64) (project.Project, error) {
	return project.Project{}, errors.New("boom")
}
func (boomRepo) ListProjectsByOwners(context.Context, []string) ([]project.Project, error) {
	return nil, errors.New("boom")
}

func boomRouter() http.Handler {
	return httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(boomRepo{Store: fake.NewStore()}),
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
