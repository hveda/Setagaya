package usageapp_test

import (
	"context"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/usageapp"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func TestRecordAndHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := usageapp.NewService(fake.NewStore())

	if err := svc.RecordStart(ctx, 1, "alice", 4, 40); err != nil {
		t.Fatalf("RecordStart: %v", err)
	}
	if err := svc.RecordFinish(ctx, 1, 40); err != nil {
		t.Fatalf("RecordFinish: %v", err)
	}

	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	hist, err := svc.History(ctx, from, to)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 1 || hist[0].Owner != "alice" {
		t.Fatalf("history = %+v", hist)
	}
}

func TestSummary_VUHByOwnerAndContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	// Fix time so billing hours are deterministic: start, then finish +90min.
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	cur := base
	store.SetNow(func() time.Time { return cur })
	svc := usageapp.NewService(store)

	// alice: 10 VU for 90 minutes -> ceil(1.5h)=2 -> 20 VUH.
	if err := svc.RecordStart(ctx, 1, "alice", 1, 10); err != nil {
		t.Fatalf("start: %v", err)
	}
	cur = base.Add(90 * time.Minute)
	if err := svc.RecordFinish(ctx, 1, 10); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// bob: 5 VU for 30 minutes -> ceil(0.5h)=1 -> 5 VUH.
	cur = base
	if err := svc.RecordStart(ctx, 2, "bob", 1, 5); err != nil {
		t.Fatalf("start: %v", err)
	}
	cur = base.Add(30 * time.Minute)
	if err := svc.RecordFinish(ctx, 2, 5); err != nil {
		t.Fatalf("finish: %v", err)
	}

	summary, err := svc.Summary(ctx, base.Add(-time.Hour), base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got := summary.VUHByOwner["alice"]["default"]; got != 20 {
		t.Fatalf("alice VUH = %v, want 20", got)
	}
	if got := summary.VUHByOwner["bob"]["default"]; got != 5 {
		t.Fatalf("bob VUH = %v, want 5", got)
	}
	if got := summary.TotalVUH["default"]; got != 25 {
		t.Fatalf("total VUH = %v, want 25", got)
	}
}
