package quotaapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func at(seconds int) time.Time { return time.Unix(int64(seconds), 0).UTC() }

func newQuotaService(t *testing.T) (*quotaapp.Service, *fake.Store) {
	t.Helper()
	store := fake.NewStore()
	return quotaapp.NewService(store), store
}

func TestReserve_AdmitsWhenUnderCeiling(t *testing.T) {
	t.Parallel()
	svc, store := newQuotaService(t)
	ctx := context.Background()
	if err := store.SetCeiling(ctx, 1, "default", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	r, err := svc.Reserve(ctx, 1, "default", 5, at(0), at(60), 100)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if r.ID == 0 || r.EngineCount != 5 {
		t.Fatalf("Reserve = %+v, want a persisted reservation for 5 engines", r)
	}
}

// The boundary itself: using exactly the remaining headroom must be
// admitted, not rejected -- "exceed" means strictly over, not at, the
// ceiling.
func TestReserve_AdmitsExactlyAtCeiling(t *testing.T) {
	t.Parallel()
	svc, store := newQuotaService(t)
	ctx := context.Background()
	if err := store.SetCeiling(ctx, 1, "default", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if _, err := svc.Reserve(ctx, 1, "default", 5, at(0), at(60), 100); err != nil {
		t.Fatalf("Reserve (first): %v", err)
	}

	if _, err := svc.Reserve(ctx, 1, "default", 5, at(0), at(60), 101); err != nil {
		t.Fatalf("Reserve (second, exactly at ceiling) = %v, want nil", err)
	}
}

func TestReserve_RejectsWhenOverCeiling(t *testing.T) {
	t.Parallel()
	svc, store := newQuotaService(t)
	ctx := context.Background()
	if err := store.SetCeiling(ctx, 1, "default", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if _, err := svc.Reserve(ctx, 1, "default", 8, at(0), at(60), 100); err != nil {
		t.Fatalf("Reserve (first): %v", err)
	}

	if _, err := svc.Reserve(ctx, 1, "default", 5, at(0), at(60), 101); !errors.Is(err, quotaapp.ErrOverQuota) {
		t.Fatalf("Reserve (second, 8+5=13 > 10) = %v, want ErrOverQuota", err)
	}
}

// An unconfigured ceiling reads as 0, so any positive reservation is
// rejected -- unconfigured means nothing runs, not unlimited.
func TestReserve_ZeroCeilingRejectsEverything(t *testing.T) {
	t.Parallel()
	svc, _ := newQuotaService(t)
	if _, err := svc.Reserve(context.Background(), 1, "default", 1, at(0), at(60), 100); !errors.Is(err, quotaapp.ErrOverQuota) {
		t.Fatalf("Reserve(unconfigured ceiling) = %v, want ErrOverQuota", err)
	}
}

// A reservation elsewhere in time must not count against a window it does
// not overlap -- the whole point of InWindow scoping the sum, not a running
// total across all time.
func TestReserve_IgnoresReservationsOutsideTheWindow(t *testing.T) {
	t.Parallel()
	svc, store := newQuotaService(t)
	ctx := context.Background()
	if err := store.SetCeiling(ctx, 1, "default", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	if _, err := svc.Reserve(ctx, 1, "default", 9, at(1000), at(1060), 100); err != nil {
		t.Fatalf("Reserve (elsewhere in time): %v", err)
	}

	if _, err := svc.Reserve(ctx, 1, "default", 9, at(0), at(60), 101); err != nil {
		t.Fatalf("Reserve (non-overlapping window) = %v, want nil", err)
	}
}

func TestReserve_RejectsInvalidWindow(t *testing.T) {
	t.Parallel()
	svc, store := newQuotaService(t)
	ctx := context.Background()
	if err := store.SetCeiling(ctx, 1, "default", 10); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}

	if _, err := svc.Reserve(ctx, 1, "default", 1, at(60), at(0), 100); !errors.Is(err, reservation.ErrWindowInvalid) {
		t.Fatalf("Reserve(end before start) = %v, want ErrWindowInvalid", err)
	}
}
