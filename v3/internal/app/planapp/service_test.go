package planapp_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/hveda/Setagaya/v3/internal/app/planapp"
	"github.com/hveda/Setagaya/v3/internal/domain/collection"
	"github.com/hveda/Setagaya/v3/internal/domain/execution"
	"github.com/hveda/Setagaya/v3/internal/ports"
	"github.com/hveda/Setagaya/v3/internal/ports/fake"
)

func newPlanService(t *testing.T) (*planapp.Service, *fake.Store, *fake.ObjectStore) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	return planapp.NewService(store, obj), store, obj
}

func TestCreate_Get_List(t *testing.T) {
	t.Parallel()
	svc, _, _ := newPlanService(t)
	ctx := context.Background()

	p, err := svc.Create(ctx, "smoke", 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("Create returned zero ID")
	}
	got, err := svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "smoke" {
		t.Fatalf("Get name = %q, want smoke", got.Name)
	}
	list, err := svc.ListByProject(ctx, 10)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByProject len = %d, want 1", len(list))
	}
}

func TestCreate_InvalidName(t *testing.T) {
	t.Parallel()
	svc, _, _ := newPlanService(t)
	if _, err := svc.Create(context.Background(), "", 10); err == nil {
		t.Fatal("Create with empty name: expected error")
	}
}

func TestFileLifecycle(t *testing.T) {
	t.Parallel()
	svc, _, obj := newPlanService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)

	// Upload a JMX (test file) and a CSV (data file).
	if err := svc.UploadFile(ctx, p.ID, "plan.jmx", bytes.NewReader([]byte("<jmx/>"))); err != nil {
		t.Fatalf("UploadFile jmx: %v", err)
	}
	if err := svc.UploadFile(ctx, p.ID, "users.csv", bytes.NewReader([]byte("a,b"))); err != nil {
		t.Fatalf("UploadFile csv: %v", err)
	}

	// The object store holds them under the plan key convention.
	if _, err := obj.Download(ctx, "plan/1/plan.jmx"); err != nil {
		t.Fatalf("object not stored at plan/1/plan.jmx: %v", err)
	}

	files, err := svc.Files(ctx, p.ID)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if files.TestFile == nil || files.TestFile.Filename != "plan.jmx" {
		t.Fatalf("TestFile = %+v, want plan.jmx", files.TestFile)
	}
	if len(files.Data) != 1 || files.Data[0].Filename != "users.csv" || files.Data[0].URL == "" {
		t.Fatalf("Data = %+v, want [users.csv] with URL", files.Data)
	}

	// Download round trip.
	got, err := svc.DownloadFile(ctx, p.ID, "users.csv")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if string(got) != "a,b" {
		t.Fatalf("DownloadFile = %q, want a,b", got)
	}

	// Duplicate upload is rejected.
	if err := svc.UploadFile(ctx, p.ID, "users.csv", bytes.NewReader([]byte("x"))); !errors.Is(err, ports.ErrFileExists) {
		t.Fatalf("duplicate UploadFile = %v, want ErrFileExists", err)
	}

	// Delete the data file; it disappears from storage and listing.
	if err := svc.DeleteFile(ctx, p.ID, "users.csv"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := obj.Download(ctx, "plan/1/users.csv"); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Fatalf("object still present after delete: %v", err)
	}
}

func TestUploadFile_InvalidFilename(t *testing.T) {
	t.Parallel()
	svc, _, _ := newPlanService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)

	for _, name := range []string{"", "sub/dir.csv", "..", "."} {
		if err := svc.UploadFile(ctx, p.ID, name, bytes.NewReader(nil)); !errors.Is(err, planapp.ErrInvalidFilename) {
			t.Fatalf("UploadFile(%q) = %v, want ErrInvalidFilename", name, err)
		}
	}
}

func TestUploadFile_UnknownPlan(t *testing.T) {
	t.Parallel()
	svc, _, _ := newPlanService(t)
	if err := svc.UploadFile(context.Background(), 999, "a.csv", bytes.NewReader(nil)); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("UploadFile(unknown plan) = %v, want ErrNotFound", err)
	}
}

func TestDelete_RefusesWhenInUse(t *testing.T) {
	t.Parallel()
	svc, store, _ := newPlanService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)

	coll, _ := collection.New("peak", 10)
	collID, _ := store.CreateCollection(ctx, coll)
	if err := store.StoreExecutionCollection(ctx, collID, false, []execution.ExecutionPlan{
		{PlanID: p.ID, Engines: 1, Concurrency: 1, Duration: 60},
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	if err := svc.Delete(ctx, p.ID); !errors.Is(err, planapp.ErrPlanInUse) {
		t.Fatalf("Delete(in use) = %v, want ErrPlanInUse", err)
	}
}

func TestDelete_RemovesFiles(t *testing.T) {
	t.Parallel()
	svc, _, obj := newPlanService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)
	_ = svc.UploadFile(ctx, p.ID, "plan.jmx", bytes.NewReader([]byte("x")))

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := obj.Download(ctx, "plan/1/plan.jmx"); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Fatalf("file survived plan delete: %v", err)
	}
	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("plan survived delete: %v", err)
	}
}
