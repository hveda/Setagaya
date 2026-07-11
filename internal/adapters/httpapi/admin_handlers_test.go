package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/adminapp"
	"github.com/heridotlife/Setagaya/internal/domain/collection"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

type noopPurger struct{}

func (noopPurger) Purge(context.Context, int64) error { return nil }

func TestAdminEndpoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	c, _ := collection.New("peak", 1)
	collID, _ := store.CreateCollection(ctx, c)
	_ = sched.DeployPlan(ctx, ports.DeploySpec{ProjectID: 1, CollectionID: collID, PlanID: 1, Engines: 1})

	h := httpapi.NewRouter(httpapi.Deps{
		Admin:         adminapp.NewService(store, sched, noopPurger{}),
		DefaultOwners: []string{"setagaya"},
	})

	if rec := do(t, h, http.MethodGet, "/api/admin/collections"); rec.Code != http.StatusOK {
		t.Fatalf("admin collections = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/admin/nodes"); rec.Code != http.StatusOK {
		t.Fatalf("admin nodes = %d", rec.Code)
	}
}
