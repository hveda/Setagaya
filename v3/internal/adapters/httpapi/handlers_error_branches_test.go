package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/hveda/Setagaya/v3/internal/adapters/httpapi"
	"github.com/hveda/Setagaya/v3/internal/app/collectionapp"
	"github.com/hveda/Setagaya/v3/internal/app/planapp"
	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/domain/collection"
	"github.com/hveda/Setagaya/v3/internal/domain/execution"
	"github.com/hveda/Setagaya/v3/internal/domain/plan"
	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
	"github.com/hveda/Setagaya/v3/internal/ports/fake"
)

var errBoom = errors.New("boom")

// failStore wraps a real fake store and forces one named method to fail, so the
// handlers' service-error (500) branches can be exercised. Seeding is done
// directly on the embedded store, so it never trips the injected failure.
type failStore struct {
	*fake.Store
	fail string
}

func (f *failStore) CreatePlan(ctx context.Context, p plan.Plan) (int64, error) {
	if f.fail == "CreatePlan" {
		return 0, errBoom
	}
	return f.Store.CreatePlan(ctx, p)
}

func (f *failStore) CreateCollection(ctx context.Context, c collection.Collection) (int64, error) {
	if f.fail == "CreateCollection" {
		return 0, errBoom
	}
	return f.Store.CreateCollection(ctx, c)
}

func (f *failStore) AddPlanFile(ctx context.Context, planID int64, filename string, isTest bool) error {
	if f.fail == "AddPlanFile" {
		return errBoom
	}
	return f.Store.AddPlanFile(ctx, planID, filename, isTest)
}

func (f *failStore) AddCollectionFile(ctx context.Context, collectionID int64, filename string) error {
	if f.fail == "AddCollectionFile" {
		return errBoom
	}
	return f.Store.AddCollectionFile(ctx, collectionID, filename)
}

func (f *failStore) PlanFilesFor(ctx context.Context, planID int64) (ports.PlanFiles, error) {
	if f.fail == "PlanFilesFor" {
		return ports.PlanFiles{}, errBoom
	}
	return f.Store.PlanFilesFor(ctx, planID)
}

func (f *failStore) CollectionFilesFor(ctx context.Context, collectionID int64) ([]string, error) {
	if f.fail == "CollectionFilesFor" {
		return nil, errBoom
	}
	return f.Store.CollectionFilesFor(ctx, collectionID)
}

func (f *failStore) StoreExecutionCollection(ctx context.Context, collectionID int64, csvSplit bool, plans []execution.ExecutionPlan) error {
	if f.fail == "StoreExecutionCollection" {
		return errBoom
	}
	return f.Store.StoreExecutionCollection(ctx, collectionID, csvSplit, plans)
}

func (f *failStore) ListPlansByProject(ctx context.Context, projectID int64) ([]plan.Plan, error) {
	if f.fail == "ListPlansByProject" {
		return nil, errBoom
	}
	return f.Store.ListPlansByProject(ctx, projectID)
}

// failEnv builds a router over a failStore and seeds an owned
// project/plan/collection. Set fs.fail before issuing the request.
func failEnv(t *testing.T) (http.Handler, *failStore, ids) {
	t.Helper()
	fs := &failStore{Store: fake.NewStore()}
	obj := fake.NewObjectStore()
	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(fs),
		Plans:         planapp.NewService(fs, obj),
		Collections:   collectionapp.NewService(fs, obj, 100),
		Store:         obj,
		DefaultOwners: []string{"setagaya"},
	})

	ctx := context.Background()
	p, _ := project.New("web", "setagaya", "")
	projectID, _ := fs.CreateProject(ctx, p)
	pl, _ := plan.New("smoke", projectID)
	planID, _ := fs.Store.CreatePlan(ctx, pl)
	c, _ := collection.New("peak", projectID)
	collID, _ := fs.Store.CreateCollection(ctx, c)
	return router, fs, ids{projectID, planID, collID}
}

type ids struct{ project, plan, collection int64 }

func TestHandlers_ServiceErrors_500(t *testing.T) {
	t.Parallel()

	t.Run("createPlan", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "CreatePlan"
		rec := postForm(t, h, "/api/plans", url.Values{"name": {"x"}, "project_id": {itoa(id.project)}})
		assert500(t, rec.Code)
	})
	t.Run("createCollection", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "CreateCollection"
		rec := postForm(t, h, "/api/collections", url.Values{"name": {"x"}, "project_id": {itoa(id.project)}})
		assert500(t, rec.Code)
	})
	t.Run("uploadPlanFile", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "AddPlanFile"
		rec := putMultipart(t, h, "/api/plans/"+itoa(id.plan)+"/files", "a.csv", "x")
		assert500(t, rec.Code)
	})
	t.Run("uploadCollectionFile", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "AddCollectionFile"
		rec := putMultipart(t, h, "/api/collections/"+itoa(id.collection)+"/files", "a.csv", "x")
		assert500(t, rec.Code)
	})
	t.Run("listPlanFiles", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "PlanFilesFor"
		rec := do(t, h, http.MethodGet, "/api/plans/"+itoa(id.plan)+"/files")
		assert500(t, rec.Code)
	})
	t.Run("listCollectionFiles", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "CollectionFilesFor"
		rec := do(t, h, http.MethodGet, "/api/collections/"+itoa(id.collection)+"/files")
		assert500(t, rec.Code)
	})
	t.Run("uploadCollectionConfig", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "StoreExecutionCollection"
		yamlCfg := "multi-test:\n  collectionid: " + itoa(id.collection) + "\n  tests:\n    - testid: " + itoa(id.plan) + "\n      concurrency: 1\n      rampup: 1\n      engines: 1\n      duration: 1\n"
		rec := putMultipart(t, h, "/api/collections/"+itoa(id.collection)+"/config", "c.yaml", yamlCfg)
		assert500(t, rec.Code)
	})
	t.Run("deleteProject", func(t *testing.T) {
		h, fs, id := failEnv(t)
		fs.fail = "ListPlansByProject"
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
