package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	"github.com/heridotlife/Setagaya/internal/app/usageapp"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

func usageRouter(t *testing.T) (http.Handler, *fake.Store) {
	t.Helper()
	store := fake.NewStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Usage:         usageapp.NewService(store),
		DefaultOwners: []string{"honryu"},
	})
	return h, store
}

func TestUsageHistoryAndSummary(t *testing.T) {
	t.Parallel()
	h, store := usageRouter(t)
	ctx := context.Background()
	if err := store.StartLaunch(ctx, 1, "alice", 2, 20); err != nil {
		t.Fatalf("StartLaunch: %v", err)
	}
	if err := store.FinishLaunch(ctx, 1, 20); err != nil {
		t.Fatalf("FinishLaunch: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/api/usage/history")
	if rec.Code != http.StatusOK {
		t.Fatalf("history = %d", rec.Code)
	}
	var hist []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}

	rec = do(t, h, http.MethodGet, "/api/usage/summary")
	if rec.Code != http.StatusOK {
		t.Fatalf("summary = %d", rec.Code)
	}
	var summary usageapp.Summary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if _, ok := summary.VUHByOwner["alice"]; !ok {
		t.Fatalf("summary missing alice: %+v", summary)
	}
}

func TestUsage_InvalidTimeParams(t *testing.T) {
	t.Parallel()
	h, _ := usageRouter(t)
	if rec := do(t, h, http.MethodGet, "/api/usage/history?from=not-a-time"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad from = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/usage/summary?to=nope"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad to = %d, want 400", rec.Code)
	}
}
