package projectapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
	"github.com/hveda/Setagaya/v3/internal/ports/fake"
)

func newService() *projectapp.Service {
	return projectapp.NewService(fake.NewProjectRepository())
}

func TestService_List_EmptyByDefault(t *testing.T) {
	t.Parallel()
	svc := newService()

	got, err := svc.List(context.Background(), []string{"team-a"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List on empty repo returned %d, want 0", len(got))
	}
}

func TestService_Create_Then_List(t *testing.T) {
	t.Parallel()
	svc := newService()
	ctx := context.Background()

	created, err := svc.Create(ctx, "web-api", "team-a", "7")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("Create returned zero ID")
	}

	got, err := svc.List(ctx, []string{"team-a"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "web-api" {
		t.Fatalf("List = %+v, want one project named web-api", got)
	}
}

func TestService_Create_ValidationErrorNotPersisted(t *testing.T) {
	t.Parallel()
	svc := newService()
	ctx := context.Background()

	_, err := svc.Create(ctx, "", "team-a", "")
	if !errors.Is(err, project.ErrNameRequired) {
		t.Fatalf("Create(empty name) err = %v, want ErrNameRequired", err)
	}

	got, err := svc.List(ctx, []string{"team-a"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("invalid Create should persist nothing, got %d projects", len(got))
	}
}

func TestService_Get_And_Delete(t *testing.T) {
	t.Parallel()
	svc := newService()
	ctx := context.Background()

	created, err := svc.Create(ctx, "web-api", "team-a", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("Get ID = %d, want %d", got.ID, created.ID)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Get after Delete err = %v, want ErrNotFound", err)
	}
}
