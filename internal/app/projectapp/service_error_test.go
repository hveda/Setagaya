package projectapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

// failingCreate embeds a real fake store but forces CreateProject to fail,
// verifying the service surfaces repository errors from Create.
type failingCreate struct {
	*fake.Store
	err error
}

func (f failingCreate) CreateProject(context.Context, project.Project) (int64, error) {
	return 0, f.err
}

func TestService_Create_PropagatesRepoError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("db exploded")
	svc := projectapp.NewService(failingCreate{Store: fake.NewStore(), err: sentinel})

	_, err := svc.Create(context.Background(), "web-api", "team-a", "1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Create err = %v, want %v", err, sentinel)
	}
}
