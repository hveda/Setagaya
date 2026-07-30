package scenarioapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/app/scenarioapp"
)

func TestDownloadFile_InvalidFilename(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	if _, err := svc.DownloadFile(context.Background(), 1, "../etc"); !errors.Is(err, scenarioapp.ErrInvalidFilename) {
		t.Fatalf("DownloadFile(bad) = %v, want ErrInvalidFilename", err)
	}
}

func TestDeleteFile_InvalidFilename(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	if err := svc.DeleteFile(context.Background(), 1, "sub/dir"); !errors.Is(err, scenarioapp.ErrInvalidFilename) {
		t.Fatalf("DeleteFile(bad) = %v, want ErrInvalidFilename", err)
	}
}

func TestDelete_MissingScenario(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	if err := svc.Delete(context.Background(), 999); err == nil {
		t.Fatal("Delete(missing) expected error")
	}
}

func TestDeleteFile_UnknownFile(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)
	if err := svc.DeleteFile(ctx, p.ID, "nope.csv"); err == nil {
		t.Fatal("DeleteFile(unknown) expected error")
	}
}
