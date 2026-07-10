// Package repositorytest provides a reusable conformance suite for the
// repository ports. The same suite runs against the in-memory fake (fast, no
// infra) and against real adapters like MySQL (behind the integration build
// tag), guaranteeing they behave identically.
package repositorytest

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/domain/project"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// NewProjectRepo returns a fresh, empty ProjectRepository for a single subtest.
// Implementations must isolate state between calls.
type NewProjectRepo func(t *testing.T) ports.ProjectRepository

// RunProjectRepositoryContract exercises the ProjectRepository behaviour that
// every implementation must satisfy.
func RunProjectRepositoryContract(t *testing.T, newRepo NewProjectRepo) {
	t.Helper()

	t.Run("CreateAssignsIDAndGetRoundTrips", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		p, err := project.New("web-api", "team-a", "42")
		if err != nil {
			t.Fatalf("build project: %v", err)
		}
		id, err := repo.CreateProject(ctx, p)
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		if id <= 0 {
			t.Fatalf("CreateProject returned id %d, want > 0", id)
		}

		got, err := repo.GetProject(ctx, id)
		if err != nil {
			t.Fatalf("GetProject: %v", err)
		}
		if got.ID != id || got.Name != "web-api" || got.Owner != "team-a" || got.SID != "42" {
			t.Fatalf("GetProject = %+v, want id=%d name=web-api owner=team-a sid=42", got, id)
		}
		if got.CreatedTime.IsZero() {
			t.Errorf("GetProject CreatedTime is zero, want a persisted timestamp")
		}
	})

	t.Run("GetMissingReturnsNotFound", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.GetProject(context.Background(), 999999)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetProject(missing) err = %v, want ErrNotFound", err)
		}
	})

	t.Run("ListByOwnersFiltersAndEmptyOwnersReturnsEmpty", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		mustCreate(t, repo, "p1", "team-a", "1")
		mustCreate(t, repo, "p2", "team-a", "2")
		mustCreate(t, repo, "p3", "team-b", "3")

		got, err := repo.ListProjectsByOwners(ctx, []string{"team-a"})
		if err != nil {
			t.Fatalf("ListProjectsByOwners: %v", err)
		}
		if names := projectNames(got); !equalStringSet(names, []string{"p1", "p2"}) {
			t.Fatalf("ListProjectsByOwners(team-a) names = %v, want [p1 p2]", names)
		}

		both, err := repo.ListProjectsByOwners(ctx, []string{"team-a", "team-b"})
		if err != nil {
			t.Fatalf("ListProjectsByOwners(both): %v", err)
		}
		if len(both) != 3 {
			t.Fatalf("ListProjectsByOwners(both) len = %d, want 3", len(both))
		}

		empty, err := repo.ListProjectsByOwners(ctx, nil)
		if err != nil {
			t.Fatalf("ListProjectsByOwners(nil): %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("ListProjectsByOwners(nil) len = %d, want 0", len(empty))
		}
	})

	t.Run("DeleteRemovesAndIsIdempotentlyNotFound", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id := mustCreate(t, repo, "p1", "team-a", "1")
		if err := repo.DeleteProject(ctx, id); err != nil {
			t.Fatalf("DeleteProject: %v", err)
		}
		if _, err := repo.GetProject(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetProject after delete err = %v, want ErrNotFound", err)
		}
		if err := repo.DeleteProject(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteProject(missing) err = %v, want ErrNotFound", err)
		}
	})
}

func mustCreate(t *testing.T, repo ports.ProjectRepository, name, owner, sid string) int64 {
	t.Helper()
	p, err := project.New(name, owner, sid)
	if err != nil {
		t.Fatalf("build project %q: %v", name, err)
	}
	id, err := repo.CreateProject(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProject %q: %v", name, err)
	}
	return id
}

func projectNames(ps []project.Project) []string {
	names := make([]string, 0, len(ps))
	for _, p := range ps {
		names = append(names, p.Name)
	}
	return names
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}
