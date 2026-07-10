package projectapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/domain/plan"
	"github.com/heridotlife/Setagaya/v3/internal/domain/project"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
	"github.com/heridotlife/Setagaya/v3/internal/ports/fake"
)

func newService() *projectapp.Service {
	return projectapp.NewService(fake.NewStore())
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

func TestService_Delete_Missing(t *testing.T) {
	t.Parallel()
	if err := newService().Delete(context.Background(), 4242); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}
}

func TestService_Delete_RefusesWhenNotEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Project with a plan cannot be deleted.
	withPlan := fake.NewStore()
	pid, _ := withPlan.CreateProject(ctx, mustProject(t, "p", "team-a"))
	_, _ = withPlan.CreatePlan(ctx, mustPlan(t, "smoke", pid))
	if err := projectapp.NewService(withPlan).Delete(ctx, pid); !errors.Is(err, projectapp.ErrProjectHasPlans) {
		t.Fatalf("Delete(project with plan) = %v, want ErrProjectHasPlans", err)
	}

	// Project with a collection cannot be deleted.
	withColl := fake.NewStore()
	pid2, _ := withColl.CreateProject(ctx, mustProject(t, "p", "team-a"))
	_, _ = withColl.CreateCollection(ctx, mustCollection(t, "peak", pid2))
	if err := projectapp.NewService(withColl).Delete(ctx, pid2); !errors.Is(err, projectapp.ErrProjectHasCollections) {
		t.Fatalf("Delete(project with collection) = %v, want ErrProjectHasCollections", err)
	}
}

func mustProject(t *testing.T, name, owner string) project.Project {
	t.Helper()
	p, err := project.New(name, owner, "")
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	return p
}

func mustPlan(t *testing.T, name string, projectID int64) plan.Plan {
	t.Helper()
	p, err := plan.New(name, projectID)
	if err != nil {
		t.Fatalf("plan.New: %v", err)
	}
	return p
}

func mustCollection(t *testing.T, name string, projectID int64) collection.Collection {
	t.Helper()
	c, err := collection.New(name, projectID)
	if err != nil {
		t.Fatalf("collection.New: %v", err)
	}
	return c
}
