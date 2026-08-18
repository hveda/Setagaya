package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

var errBoom = errors.New("boom")

// failStore wraps a real fake store and forces one named method to fail, so the
// handlers' service-error (500) branches can be exercised. Seeding is done
// directly on the embedded store, so it never trips the injected failure.
type failStore struct {
	*fake.Store
	fail string
}

func (f *failStore) CreateScenario(ctx context.Context, p scenario.Scenario) (int64, error) {
	if f.fail == "CreateScenario" {
		return 0, errBoom
	}
	return f.Store.CreateScenario(ctx, p)
}

func (f *failStore) CreateExecution(ctx context.Context, c execution.Execution) (int64, error) {
	if f.fail == "CreateExecution" {
		return 0, errBoom
	}
	return f.Store.CreateExecution(ctx, c)
}

func (f *failStore) AddScenarioFile(ctx context.Context, scenarioID int64, filename string, isTest bool) error {
	if f.fail == "AddScenarioFile" {
		return errBoom
	}
	return f.Store.AddScenarioFile(ctx, scenarioID, filename, isTest)
}

func (f *failStore) AddExecutionFile(ctx context.Context, executionID int64, filename string) error {
	if f.fail == "AddExecutionFile" {
		return errBoom
	}
	return f.Store.AddExecutionFile(ctx, executionID, filename)
}

func (f *failStore) ScenarioFilesFor(ctx context.Context, scenarioID int64) (ports.ScenarioFiles, error) {
	if f.fail == "ScenarioFilesFor" {
		return ports.ScenarioFiles{}, errBoom
	}
	return f.Store.ScenarioFilesFor(ctx, scenarioID)
}

func (f *failStore) ExecutionFilesFor(ctx context.Context, executionID int64) ([]string, error) {
	if f.fail == "ExecutionFilesFor" {
		return nil, errBoom
	}
	return f.Store.ExecutionFilesFor(ctx, executionID)
}

func (f *failStore) StoreExecutionConfig(ctx context.Context, executionID int64, csvSplit bool, entries []loadprofile.Entry, criteria []string) error {
	if f.fail == "StoreExecutionConfig" {
		return errBoom
	}
	return f.Store.StoreExecutionConfig(ctx, executionID, csvSplit, entries, criteria)
}

func (f *failStore) ListScenariosByProject(ctx context.Context, projectID int64) ([]scenario.Scenario, error) {
	if f.fail == "ListScenariosByProject" {
		return nil, errBoom
	}
	return f.Store.ListScenariosByProject(ctx, projectID)
}

// failEnv builds a router over a failStore and seeds an owned
// project/scenario/execution. Set fs.fail before issuing the request.
func failEnv(t *testing.T) (http.Handler, *failStore, ids) {
	t.Helper()
	fs := &failStore{Store: fake.NewStore()}
	obj := fake.NewObjectStore()
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(fs),
		Scenarios:     scenarioapp.NewService(fs, obj),
		Executions:    executionapp.NewService(fs, obj, 100),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})

	ctx := context.Background()
	p, _ := project.New("web", "honryu", "")
	projectID, _ := fs.CreateProject(ctx, p)
	pl, _ := scenario.New("smoke", projectID)
	scenarioID, _ := fs.Store.CreateScenario(ctx, pl)
	c, _ := execution.New("peak", projectID)
	collID, _ := fs.Store.CreateExecution(ctx, c)
	return router, fs, ids{projectID, scenarioID, collID}
}

type ids struct{ project, scenario, execution int64 }

func TestHandlers_ServiceErrors_500(t *testing.T) {
	t.Parallel()

	t.Run("createScenario", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "CreateScenario"
		rec := postForm(t, h, "/api/scenarios", url.Values{"name": {"x"}, "project_id": {itoa(id.project)}})
		assert500(t, rec.Code)
	})
	t.Run("createExecution", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "CreateExecution"
		rec := postForm(t, h, "/api/executions", url.Values{"name": {"x"}, "project_id": {itoa(id.project)}})
		assert500(t, rec.Code)
	})
	t.Run("uploadScenarioFile", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "AddScenarioFile"
		rec := putMultipart(t, h, "/api/scenarios/"+itoa(id.scenario)+"/files", "a.csv", "x")
		assert500(t, rec.Code)
	})
	t.Run("uploadExecutionFile", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "AddExecutionFile"
		rec := putMultipart(t, h, "/api/executions/"+itoa(id.execution)+"/files", "a.csv", "x")
		assert500(t, rec.Code)
	})
	t.Run("listScenarioFiles", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "ScenarioFilesFor"
		rec := do(t, h, http.MethodGet, "/api/scenarios/"+itoa(id.scenario)+"/files")
		assert500(t, rec.Code)
	})
	t.Run("listExecutionFiles", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "ExecutionFilesFor"
		rec := do(t, h, http.MethodGet, "/api/executions/"+itoa(id.execution)+"/files")
		assert500(t, rec.Code)
	})
	t.Run("uploadExecutionConfig", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "StoreExecutionConfig"
		yamlCfg := "multi-test:\n  collectionid: " + itoa(id.execution) + "\n  tests:\n    - testid: " + itoa(id.scenario) + "\n      concurrency: 1\n      rampup: 1\n      engines: 1\n      duration: 1\n"
		rec := putMultipart(t, h, "/api/executions/"+itoa(id.execution)+"/config", "c.yaml", yamlCfg)
		assert500(t, rec.Code)
	})
	t.Run("deleteProject", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "ListScenariosByProject"
		rec := do(t, h, http.MethodDelete, "/api/projects/"+itoa(id.project))
		assert500(t, rec.Code)
	})
}

func assert500(t *testing.T, code int) {
	t.Helper()
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", code)
	}
}
