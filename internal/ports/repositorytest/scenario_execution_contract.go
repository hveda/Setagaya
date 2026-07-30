package repositorytest

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// Repository is the full repository surface implemented by both the fake and
// the MySQL adapter. Cross-aggregate behaviour (e.g. "is this scenario in use") is
// tested against this combined interface.
type Repository interface {
	ports.ProjectRepository
	ports.ScenarioRepository
	ports.ExecutionRepository
}

// NewRepo returns a fresh, empty Repository for a single subtest.
type NewRepo func(t *testing.T) Repository

// RunScenarioRepositoryContract exercises ScenarioRepository behaviour.
func RunScenarioRepositoryContract(t *testing.T, newRepo NewRepo) {
	t.Helper()

	// Portability decides which engines a scenario may run on. If it does not
	// survive a round trip the domain refuses every engine, so the contract
	// pins it for the fake and every real adapter alike.
	t.Run("PortabilityRoundTrips", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		portable, err := scenario.New("portable", 10)
		if err != nil {
			t.Fatalf("scenario.New: %v", err)
		}
		portableID, err := repo.CreateScenario(ctx, portable)
		if err != nil {
			t.Fatalf("CreateScenario(portable): %v", err)
		}

		native, err := scenario.NewNative("imported", 10, taurus.ExecutorJMeter)
		if err != nil {
			t.Fatalf("scenario.NewNative: %v", err)
		}
		nativeID, err := repo.CreateScenario(ctx, native)
		if err != nil {
			t.Fatalf("CreateScenario(native): %v", err)
		}

		gotPortable, err := repo.GetScenario(ctx, portableID)
		if err != nil {
			t.Fatalf("GetScenario(portable): %v", err)
		}
		if gotPortable.Kind != scenario.KindPortable || gotPortable.Engine != "" {
			t.Errorf("portable round trip = kind %q engine %q, want portable with no engine",
				gotPortable.Kind, gotPortable.Engine)
		}
		if err := gotPortable.Validate(); err != nil {
			t.Errorf("portable round trip does not validate: %v", err)
		}

		gotNative, err := repo.GetScenario(ctx, nativeID)
		if err != nil {
			t.Fatalf("GetScenario(native): %v", err)
		}
		if gotNative.Kind != scenario.KindNative || gotNative.Engine != taurus.ExecutorJMeter {
			t.Errorf("native round trip = kind %q engine %q, want native/jmeter",
				gotNative.Kind, gotNative.Engine)
		}
		if err := gotNative.CanRunOn(taurus.ExecutorK6); err == nil {
			t.Error("a JMeter-pinned scenario read back accepted k6")
		}

		// Listing must carry portability too: engine selection is offered from
		// list views, not only from a single fetch.
		listed, err := repo.ListScenariosByProject(ctx, 10)
		if err != nil {
			t.Fatalf("ListScenariosByProject: %v", err)
		}
		for _, s := range listed {
			if err := s.Validate(); err != nil {
				t.Errorf("listed scenario %q does not validate: %v", s.Name, err)
			}
		}
	})

	t.Run("CreateGetListDelete", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id := mustCreateScenario(t, repo, "smoke", 10)
		got, err := repo.GetScenario(ctx, id)
		if err != nil {
			t.Fatalf("GetScenario: %v", err)
		}
		if got.ID != id || got.Name != "smoke" || got.ProjectID != 10 || got.CreatedTime.IsZero() {
			t.Fatalf("GetScenario = %+v, want id=%d name=smoke project=10 with timestamp", got, id)
		}

		mustCreateScenario(t, repo, "second", 10)
		mustCreateScenario(t, repo, "other-project", 99)
		inProject, err := repo.ListScenariosByProject(ctx, 10)
		if err != nil {
			t.Fatalf("ListScenariosByProject: %v", err)
		}
		if names := planNames(inProject); !equalStringSet(names, []string{"smoke", "second"}) {
			t.Fatalf("ListScenariosByProject(10) = %v, want [smoke second]", names)
		}

		if err := repo.DeleteScenario(ctx, id); err != nil {
			t.Fatalf("DeleteScenario: %v", err)
		}
		if _, err := repo.GetScenario(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetScenario after delete = %v, want ErrNotFound", err)
		}
		if err := repo.DeleteScenario(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteScenario(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("GetMissingReturnsNotFound", func(t *testing.T) {
		repo := newRepo(t)
		if _, err := repo.GetScenario(context.Background(), 987654); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetScenario(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("Files", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		id := mustCreateScenario(t, repo, "smoke", 10)

		if err := repo.AddScenarioFile(ctx, id, "test.jmx", true); err != nil {
			t.Fatalf("AddScenarioFile(test): %v", err)
		}
		if err := repo.AddScenarioFile(ctx, id, "users.csv", false); err != nil {
			t.Fatalf("AddScenarioFile(data): %v", err)
		}

		// One JMX slot per scenario; a second test file conflicts.
		if err := repo.AddScenarioFile(ctx, id, "other.jmx", true); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("AddScenarioFile(second test) = %v, want ErrFileExists", err)
		}
		if err := repo.AddScenarioFile(ctx, id, "users.csv", false); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("AddScenarioFile(dup data) = %v, want ErrFileExists", err)
		}

		files, err := repo.ScenarioFilesFor(ctx, id)
		if err != nil {
			t.Fatalf("ScenarioFilesFor: %v", err)
		}
		if files.TestFile != "test.jmx" || !equalStringSet(files.Data, []string{"users.csv"}) {
			t.Fatalf("ScenarioFilesFor = %+v, want test.jmx + [users.csv]", files)
		}

		if err := repo.DeleteScenarioFile(ctx, id, "users.csv", false); err != nil {
			t.Fatalf("DeleteScenarioFile(data): %v", err)
		}
		if err := repo.DeleteScenarioFile(ctx, id, "users.csv", false); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteScenarioFile(missing) = %v, want ErrNotFound", err)
		}
		if err := repo.DeleteScenarioFile(ctx, id, "test.jmx", true); err != nil {
			t.Fatalf("DeleteScenarioFile(test): %v", err)
		}
	})

	t.Run("ScenarioInUse", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		scenarioID := mustCreateScenario(t, repo, "smoke", 10)

		inUse, err := repo.ScenarioInUse(ctx, scenarioID)
		if err != nil {
			t.Fatalf("ScenarioInUse: %v", err)
		}
		if inUse {
			t.Fatal("ScenarioInUse = true for an unused scenario")
		}

		collID := mustCreateExecution(t, repo, "peak", 10)
		if err := repo.StoreLoadProfile(ctx, collID, false, []loadprofile.Entry{
			{Name: "smoke", ScenarioID: scenarioID, Engines: 1, Concurrency: 1, Duration: 60},
		}); err != nil {
			t.Fatalf("StoreLoadProfile: %v", err)
		}

		inUse, err = repo.ScenarioInUse(ctx, scenarioID)
		if err != nil {
			t.Fatalf("ScenarioInUse: %v", err)
		}
		if !inUse {
			t.Fatal("ScenarioInUse = false after the scenario was added to an execution")
		}
	})
}

// RunExecutionRepositoryContract exercises ExecutionRepository behaviour.
func RunExecutionRepositoryContract(t *testing.T, newRepo NewRepo) {
	t.Helper()

	t.Run("CreateGetListDelete", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		id := mustCreateExecution(t, repo, "peak", 10)
		got, err := repo.GetExecution(ctx, id)
		if err != nil {
			t.Fatalf("GetExecution: %v", err)
		}
		if got.ID != id || got.Name != "peak" || got.ProjectID != 10 || got.CSVSplit || got.CreatedTime.IsZero() {
			t.Fatalf("GetExecution = %+v, want id=%d name=peak project=10 csv_split=false with timestamp", got, id)
		}

		mustCreateExecution(t, repo, "other", 99)
		inProject, err := repo.ListExecutionsByProject(ctx, 10)
		if err != nil {
			t.Fatalf("ListExecutionsByProject: %v", err)
		}
		if len(inProject) != 1 || inProject[0].Name != "peak" {
			t.Fatalf("ListExecutionsByProject(10) = %+v, want only peak", inProject)
		}

		if err := repo.DeleteExecution(ctx, id); err != nil {
			t.Fatalf("DeleteExecution: %v", err)
		}
		if err := repo.DeleteExecution(ctx, id); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteExecution(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("Files", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		id := mustCreateExecution(t, repo, "peak", 10)

		if err := repo.AddExecutionFile(ctx, id, "shared.csv"); err != nil {
			t.Fatalf("AddExecutionFile: %v", err)
		}
		if err := repo.AddExecutionFile(ctx, id, "shared.csv"); !errors.Is(err, ports.ErrFileExists) {
			t.Fatalf("AddExecutionFile(dup) = %v, want ErrFileExists", err)
		}
		files, err := repo.ExecutionFilesFor(ctx, id)
		if err != nil {
			t.Fatalf("ExecutionFilesFor: %v", err)
		}
		if !equalStringSet(files, []string{"shared.csv"}) {
			t.Fatalf("ExecutionFilesFor = %v, want [shared.csv]", files)
		}
		if err := repo.DeleteExecutionFile(ctx, id, "shared.csv"); err != nil {
			t.Fatalf("DeleteExecutionFile: %v", err)
		}
		if err := repo.DeleteExecutionFile(ctx, id, "shared.csv"); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("DeleteExecutionFile(missing) = %v, want ErrNotFound", err)
		}
	})

	t.Run("ExecutionScenariosStoreReplacesAndSetsCSVSplit", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		id := mustCreateExecution(t, repo, "peak", 10)

		first := []loadprofile.Entry{
			{Name: "a", ScenarioID: 1, Engines: 2, Concurrency: 10, Duration: 60},
			{Name: "b", ScenarioID: 2, Engines: 3, Concurrency: 10, Duration: 60},
		}
		if err := repo.StoreLoadProfile(ctx, id, true, first); err != nil {
			t.Fatalf("StoreLoadProfile(first): %v", err)
		}
		got, err := repo.LoadProfileFor(ctx, id)
		if err != nil {
			t.Fatalf("LoadProfileFor: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("LoadProfileFor len = %d, want 2", len(got))
		}
		if c, _ := repo.GetExecution(ctx, id); !c.CSVSplit {
			t.Fatalf("execution CSVSplit = false after store, want true")
		}

		// Storing a smaller set replaces (not merges) the previous scenarios.
		second := []loadprofile.Entry{{Name: "a", ScenarioID: 1, Engines: 5, Concurrency: 20, Duration: 60}}
		if err := repo.StoreLoadProfile(ctx, id, false, second); err != nil {
			t.Fatalf("StoreLoadProfile(second): %v", err)
		}
		got, _ = repo.LoadProfileFor(ctx, id)
		if len(got) != 1 || got[0].ScenarioID != 1 || got[0].Engines != 5 {
			t.Fatalf("LoadProfileFor after replace = %+v, want single scenario 1 with 5 engines", got)
		}
		if c, _ := repo.GetExecution(ctx, id); c.CSVSplit {
			t.Fatalf("execution CSVSplit = true after second store, want false")
		}
	})

	t.Run("StoreOnMissingExecution", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.StoreLoadProfile(context.Background(), 987654, false,
			[]loadprofile.Entry{{ScenarioID: 1, Engines: 1, Concurrency: 1, Duration: 1}})
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("StoreLoadProfile(missing execution) = %v, want ErrNotFound", err)
		}
	})
}

func mustCreateScenario(t *testing.T, repo Repository, name string, projectID int64) int64 {
	t.Helper()
	p, err := scenario.New(name, projectID)
	if err != nil {
		t.Fatalf("build scenario %q: %v", name, err)
	}
	id, err := repo.CreateScenario(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateScenario %q: %v", name, err)
	}
	return id
}

func mustCreateExecution(t *testing.T, repo Repository, name string, projectID int64) int64 {
	t.Helper()
	c, err := execution.New(name, projectID)
	if err != nil {
		t.Fatalf("build execution %q: %v", name, err)
	}
	id, err := repo.CreateExecution(context.Background(), c)
	if err != nil {
		t.Fatalf("CreateExecution %q: %v", name, err)
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
