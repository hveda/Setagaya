package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/adminapp"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
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
	c, _ := execution.New("peak", 1)
	collID, _ := store.CreateExecution(ctx, c)
	_ = sched.DeployScenario(ctx, ports.DeploySpec{ProjectID: 1, ExecutionID: collID, ScenarioID: 1, Engines: 1})

	h := httpapi.NewRouter(httpapi.Deps{
		Admin:         adminapp.NewService(store, sched, noopPurger{}),
		DefaultOwners: []string{"honryu"},
	})

	if rec := do(t, h, http.MethodGet, "/api/admin/executions"); rec.Code != http.StatusOK {
		t.Fatalf("admin executions = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/admin/nodes"); rec.Code != http.StatusOK {
		t.Fatalf("admin nodes = %d", rec.Code)
	}
}
