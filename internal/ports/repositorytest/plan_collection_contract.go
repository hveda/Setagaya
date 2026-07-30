package repositorytest

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Repository is the full repository surface implemented by both the fake and
// the MySQL adapter. Cross-aggregate behaviour (e.g. "is this plan in use") is
// tested against this combined interface.
type Repository interface {
	ports.ProjectRepository
	ports.PlanRepository
	ports.CollectionRepository
}

// NewRepo returns a fresh, empty Repository for a single subtest.
type NewRepo func(t *testing.T) Repository

// RunPlanRepositoryContract exercises PlanRepository behaviour.
func RunPlanRepositoryContract(t *testing.T, newRepo NewRepo) {
	t.Helper()

	t.Run("CreateGetListDelete", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id := mustCreatePlan(t, repo, "smoke", 10)
		got, err := repo.GetPlan(ctx, id)
		if err != nil {
			t.Fatalf("GetPlan: %v", err)
		}
		if got.ID != id || got.Name != "smoke" || got.ProjectID != 10 || got.CreatedTime.IsZero() {
			t.Fatalf("GetPlan = %+v, want id=%d name=smoke project=10 with timestamp", got, id)
		}

		mustCreatePlan(t, repo, "second", 10)
		mustCreatePlan(t, repo, "other-project", 99)
		inProject, err := repo.ListPlansByProject(ctx, 10)
		if err != nil {
			t.Fatalf("ListPlansByProject: %v", err)
		}
		if names := planNames(inProject); !equalStringSet(names, []string{"smoke", "second"}) {
			t.Fatalf("ListPlansByProject(10) = %v, want [smoke second]", names)
		}

		if err := repo.DeletePlan(ctx, id); err != nil {
			t.Fatalf("DeletePlan: %v", err)
		}
		if _, err := repo.GetPlan(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetPlan after delete = %v, want ErrNotFound", err)
		}
		if err := repo.DeletePlan(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeletePlan(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("GetMissingReturnsNotFound", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.GetPlan(context.Background(), 987654); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetPlan(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("Files", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		id := mustCreatePlan(t, repo, "smoke", 10)

		if err := repo.AddPlanFile(ctx, id, "test.jmx", true); err != nil {
			t.Fatalf("AddPlanFile(test): %v", err)
		}
		if err := repo.AddPlanFile(ctx, id, "users.csv", false); err != nil {
			t.Fatalf("AddPlanFile(data): %v", err)
		}

		// One JMX slot per plan; a second test file conflicts.
		if err := repo.AddPlanFile(ctx, id, "other.jmx", true); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("AddPlanFile(second test) = %v, want ErrFileExists", err)
		}
		if err := repo.AddPlanFile(ctx, id, "users.csv", false); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("AddPlanFile(dup data) = %v, want ErrFileExists", err)
		}

		files, err := repo.PlanFilesFor(ctx, id)
		if err != nil {
			t.Fatalf("PlanFilesFor: %v", err)
		}
		if files.TestFile != "test.jmx" || !equalStringSet(files.Data, []string{"users.csv"}) {
			t.Fatalf("PlanFilesFor = %+v, want test.jmx + [users.csv]", files)
		}

		if err := repo.DeletePlanFile(ctx, id, "users.csv", false); err != nil {
			t.Fatalf("DeletePlanFile(data): %v", err)
		}
		if err := repo.DeletePlanFile(ctx, id, "users.csv", false); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeletePlanFile(missing) = %v, want ErrNotFound", err)
		}
		if err := repo.DeletePlanFile(ctx, id, "test.jmx", true); err != nil {
			t.Fatalf("DeletePlanFile(test): %v", err)
		}
	})

	t.Run("PlanInUse", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		planID := mustCreatePlan(t, repo, "smoke", 10)

		inUse, err := repo.PlanInUse(ctx, planID)
		if err != nil {
			t.Fatalf("PlanInUse: %v", err)
		}
		if inUse {
			t.Fatal("PlanInUse = true for an unused plan")
		}

		collID := mustCreateCollection(t, repo, "peak", 10)
		if err := repo.StoreExecutionCollection(ctx, collID, false, []loadprofile.Entry{
			{Name: "smoke", PlanID: planID, Engines: 1, Concurrency: 1, Duration: 60},
		}); err != nil {
			t.Fatalf("StoreExecutionCollection: %v", err)
		}

		inUse, err = repo.PlanInUse(ctx, planID)
		if err != nil {
			t.Fatalf("PlanInUse: %v", err)
		}
		if !inUse {
			t.Fatal("PlanInUse = false after the plan was added to a collection")
		}
	})
}

// RunCollectionRepositoryContract exercises CollectionRepository behaviour.
func RunCollectionRepositoryContract(t *testing.T, newRepo NewRepo) {
	t.Helper()

	t.Run("CreateGetListDelete", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id := mustCreateCollection(t, repo, "peak", 10)
		got, err := repo.GetCollection(ctx, id)
		if err != nil {
			t.Fatalf("GetCollection: %v", err)
		}
		if got.ID != id || got.Name != "peak" || got.ProjectID != 10 || got.CSVSplit || got.CreatedTime.IsZero() {
			t.Fatalf("GetCollection = %+v, want id=%d name=peak project=10 csv_split=false with timestamp", got, id)
		}

		mustCreateCollection(t, repo, "other", 99)
		inProject, err := repo.ListCollectionsByProject(ctx, 10)
		if err != nil {
			t.Fatalf("ListCollectionsByProject: %v", err)
		}
		if len(inProject) != 1 || inProject[0].Name != "peak" {
			t.Fatalf("ListCollectionsByProject(10) = %+v, want only peak", inProject)
		}

		if err := repo.DeleteCollection(ctx, id); err != nil {
			t.Fatalf("DeleteCollection: %v", err)
		}
		if err := repo.DeleteCollection(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteCollection(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("Files", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		id := mustCreateCollection(t, repo, "peak", 10)

		if err := repo.AddCollectionFile(ctx, id, "shared.csv"); err != nil {
			t.Fatalf("AddCollectionFile: %v", err)
		}
		if err := repo.AddCollectionFile(ctx, id, "shared.csv"); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("AddCollectionFile(dup) = %v, want ErrFileExists", err)
		}
		files, err := repo.CollectionFilesFor(ctx, id)
		if err != nil {
			t.Fatalf("CollectionFilesFor: %v", err)
		}
		if !equalStringSet(files, []string{"shared.csv"}) {
			t.Fatalf("CollectionFilesFor = %v, want [shared.csv]", files)
		}
		if err := repo.DeleteCollectionFile(ctx, id, "shared.csv"); err != nil {
			t.Fatalf("DeleteCollectionFile: %v", err)
		}
		if err := repo.DeleteCollectionFile(ctx, id, "shared.csv"); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteCollectionFile(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("ExecutionPlansStoreReplacesAndSetsCSVSplit", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		id := mustCreateCollection(t, repo, "peak", 10)

		first := []loadprofile.Entry{
			{Name: "a", PlanID: 1, Engines: 2, Concurrency: 10, Duration: 60},
			{Name: "b", PlanID: 2, Engines: 3, Concurrency: 10, Duration: 60},
		}
		if err := repo.StoreExecutionCollection(ctx, id, true, first); err != nil {
			t.Fatalf("StoreExecutionCollection(first): %v", err)
		}
		got, err := repo.ExecutionPlansFor(ctx, id)
		if err != nil {
			t.Fatalf("ExecutionPlansFor: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ExecutionPlansFor len = %d, want 2", len(got))
		}
		if c, _ := repo.GetCollection(ctx, id); !c.CSVSplit {
			t.Fatalf("collection CSVSplit = false after store, want true")
		}

		// Storing a smaller set replaces (not merges) the previous plans.
		second := []loadprofile.Entry{{Name: "a", PlanID: 1, Engines: 5, Concurrency: 20, Duration: 60}}
		if err := repo.StoreExecutionCollection(ctx, id, false, second); err != nil {
			t.Fatalf("StoreExecutionCollection(second): %v", err)
		}
		got, _ = repo.ExecutionPlansFor(ctx, id)
		if len(got) != 1 || got[0].PlanID != 1 || got[0].Engines != 5 {
			t.Fatalf("ExecutionPlansFor after replace = %+v, want single plan 1 with 5 engines", got)
		}
		if c, _ := repo.GetCollection(ctx, id); c.CSVSplit {
			t.Fatalf("collection CSVSplit = true after second store, want false")
		}
	})

	t.Run("StoreOnMissingCollection", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.StoreExecutionCollection(context.Background(), 987654, false,
			[]loadprofile.Entry{{PlanID: 1, Engines: 1, Concurrency: 1, Duration: 1}})
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("StoreExecutionCollection(missing collection) = %v, want ErrNotFound", err)
		}
	})
}

func mustCreatePlan(t *testing.T, repo Repository, name string, projectID int64) int64 {
	t.Helper()
	p, err := scenario.New(name, projectID)
	if err != nil {
		t.Fatalf("build plan %q: %v", name, err)
	}
	id, err := repo.CreatePlan(context.Background(), p)
	if err != nil {
		t.Fatalf("CreatePlan %q: %v", name, err)
	}
	return id
}

func mustCreateCollection(t *testing.T, repo Repository, name string, projectID int64) int64 {
	t.Helper()
	c, err := execution.New(name, projectID)
	if err != nil {
		t.Fatalf("build collection %q: %v", name, err)
	}
	id, err := repo.CreateCollection(context.Background(), c)
	if err != nil {
		t.Fatalf("CreateCollection %q: %v", name, err)
	}
	return id
}

func planNames(ps []scenario.Scenario) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}
