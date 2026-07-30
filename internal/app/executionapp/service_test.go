package executionapp_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

const maxEngines = 5

func newCollService(t *testing.T) (*executionapp.Service, *fake.Store, *fake.ObjectStore) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	return executionapp.NewService(store, obj, maxEngines), store, obj
}

// seedScenario creates a scenario directly in the store for the given project.
func seedScenario(t *testing.T, store *fake.Store, name string, projectID int64) int64 {
	t.Helper()
	p, err := scenario.New(name, projectID)
	if err != nil {
		t.Fatalf("scenario.New: %v", err)
	}
	id, err := store.CreateScenario(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	return id
}

func TestCreate_Get_List(t *testing.T) {
	t.Parallel()
	svc, _, _ := newCollService(t)
	ctx := context.Background()

	c, err := svc.Create(ctx, "peak", 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "peak" || got.ProjectID != 10 {
		t.Fatalf("Get = %+v, want peak/10", got)
	}
	list, err := svc.ListByProject(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByProject = %v (err %v), want 1", list, err)
	}
}

func TestFileLifecycle(t *testing.T) {
	t.Parallel()
	svc, _, obj := newCollService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, "peak", 10)

	if err := svc.UploadFile(ctx, c.ID, "shared.csv", bytes.NewReader([]byte("x,y"))); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if _, err := obj.Download(ctx, "execution/1/shared.csv"); err != nil {
		t.Fatalf("object not stored: %v", err)
	}
	files, err := svc.Files(ctx, c.ID)
	if err != nil || len(files) != 1 || files[0].Filename != "shared.csv" || files[0].URL == "" {
		t.Fatalf("Files = %+v (err %v)", files, err)
	}
	got, err := svc.DownloadFile(ctx, c.ID, "shared.csv")
	if err != nil || string(got) != "x,y" {
		t.Fatalf("DownloadFile = %q (err %v)", got, err)
	}
	if err := svc.UploadFile(ctx, c.ID, "shared.csv", bytes.NewReader([]byte("z"))); !errors.Is(err, ports.ErrFileExists) {
		t.Fatalf("dup UploadFile = %v, want ErrFileExists", err)
	}
	if err := svc.DeleteFile(ctx, c.ID, "shared.csv"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := obj.Download(ctx, "execution/1/shared.csv"); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Fatalf("object survived delete: %v", err)
	}
}

func TestDelete_RemovesFiles(t *testing.T) {
	t.Parallel()
	svc, _, obj := newCollService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, "peak", 10)
	_ = svc.UploadFile(ctx, c.ID, "shared.csv", bytes.NewReader([]byte("x")))

	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := obj.Download(ctx, "execution/1/shared.csv"); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Fatalf("file survived execution delete: %v", err)
	}
}

func TestStoreConfig_And_GetConfig(t *testing.T) {
	t.Parallel()
	svc, store, _ := newCollService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, "peak", 10)
	scenarioID := seedScenario(t, store, "smoke", 10)

	ec := loadprofile.Profile{
		ExecutionID: c.ID,
		CSVSplit:    true,
		Tests: []loadprofile.Entry{
			{ScenarioID: scenarioID, Engines: 2, Concurrency: 10, Duration: 60},
		},
	}
	if err := svc.StoreConfig(ctx, c.ID, ec); err != nil {
		t.Fatalf("StoreConfig: %v", err)
	}

	wrapper, err := svc.GetConfig(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if wrapper.Content.ExecutionID != c.ID || !wrapper.Content.CSVSplit {
		t.Fatalf("GetConfig content = %+v", wrapper.Content)
	}
	if len(wrapper.Content.Tests) != 1 || wrapper.Content.Tests[0].ScenarioID != scenarioID || wrapper.Content.Tests[0].Engines != 2 {
		t.Fatalf("GetConfig tests = %+v", wrapper.Content.Tests)
	}
}

func TestStoreConfig_Errors(t *testing.T) {
	t.Parallel()
	svc, store, _ := newCollService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, "peak", 10)
	scenarioID := seedScenario(t, store, "smoke", 10)
	foreignScenario := seedScenario(t, store, "other", 99)

	valid := func() loadprofile.Entry {
		return loadprofile.Entry{ScenarioID: scenarioID, Engines: 2, Concurrency: 10, Duration: 60}
	}

	t.Run("execution id mismatch", func(t *testing.T) {
		ec := loadprofile.Profile{ExecutionID: c.ID + 100, Tests: []loadprofile.Entry{valid()}}
		if err := svc.StoreConfig(ctx, c.ID, ec); !errors.Is(err, executionapp.ErrExecutionMismatch) {
			t.Fatalf("= %v, want ErrExecutionMismatch", err)
		}
	})

	t.Run("validation error (zero engines)", func(t *testing.T) {
		ec := loadprofile.Profile{ExecutionID: c.ID, Tests: []loadprofile.Entry{
			{ScenarioID: scenarioID, Engines: 0, Concurrency: 1, Duration: 1},
		}}
		if err := svc.StoreConfig(ctx, c.ID, ec); !errors.Is(err, loadprofile.ErrEnginesInvalid) {
			t.Fatalf("= %v, want ErrEnginesInvalid", err)
		}
	})

	t.Run("unknown scenario", func(t *testing.T) {
		ec := loadprofile.Profile{ExecutionID: c.ID, Tests: []loadprofile.Entry{
			{ScenarioID: 987654, Engines: 1, Concurrency: 1, Duration: 1},
		}}
		if err := svc.StoreConfig(ctx, c.ID, ec); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("= %v, want ErrNotFound", err)
		}
	})

	t.Run("scenario in another project", func(t *testing.T) {
		ec := loadprofile.Profile{ExecutionID: c.ID, Tests: []loadprofile.Entry{
			{ScenarioID: foreignScenario, Engines: 1, Concurrency: 1, Duration: 1},
		}}
		if err := svc.StoreConfig(ctx, c.ID, ec); !errors.Is(err, executionapp.ErrScenarioNotInProject) {
			t.Fatalf("= %v, want ErrScenarioNotInProject", err)
		}
	})

	t.Run("engine limit exceeded", func(t *testing.T) {
		ec := loadprofile.Profile{ExecutionID: c.ID, Tests: []loadprofile.Entry{
			{ScenarioID: scenarioID, Engines: maxEngines + 1, Concurrency: 1, Duration: 1},
		}}
		if err := svc.StoreConfig(ctx, c.ID, ec); !errors.Is(err, executionapp.ErrEngineLimit) {
			t.Fatalf("= %v, want ErrEngineLimit", err)
		}
	})

	t.Run("missing execution", func(t *testing.T) {
		ec := loadprofile.Profile{ExecutionID: 424242, Tests: []loadprofile.Entry{valid()}}
		if err := svc.StoreConfig(ctx, 424242, ec); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("= %v, want ErrNotFound", err)
		}
	})
}
