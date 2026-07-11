package local_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/storage/local"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/objectstoretest"
)

func TestLocalStore_Contract(t *testing.T) {
	t.Parallel()
	objectstoretest.RunObjectStoreContract(t, func(t *testing.T) ports.ObjectStore {
		return local.New(t.TempDir(), "")
	})
}

func TestLocalStore_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	store := local.New(t.TempDir(), "")
	ctx := context.Background()

	if err := store.Upload(ctx, "../escape.txt", bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("Upload with traversal key: expected error, got nil")
	}
	if _, err := store.Download(ctx, "../../etc/passwd"); err == nil {
		t.Fatal("Download with traversal key: expected error, got nil")
	}
}

func TestLocalStore_UnusableRoot(t *testing.T) {
	t.Parallel()
	// Root sits beneath a regular file, so it can never be created: every
	// operation must surface the error rather than panic or silently succeed.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := local.New(filepath.Join(blocker, "root"), "")
	ctx := context.Background()

	if err := store.Upload(ctx, "k", bytes.NewReader([]byte("x"))); err == nil {
		t.Error("Upload with unusable root: expected error, got nil")
	}
	if _, err := store.Download(ctx, "k"); err == nil {
		t.Error("Download with unusable root: expected error, got nil")
	}
	if err := store.Delete(ctx, "k"); err == nil {
		t.Error("Delete with unusable root: expected error, got nil")
	}
}

func TestLocalStore_URL(t *testing.T) {
	t.Parallel()

	withBase := local.New(t.TempDir(), "https://cdn.example.com/bucket/")
	if got := withBase.URL("plan/1/a.jmx"); got != "https://cdn.example.com/bucket/plan/1/a.jmx" {
		t.Errorf("URL with base = %q", got)
	}

	noBase := local.New(t.TempDir(), "")
	if got := noBase.URL("plan/1/a.jmx"); !strings.HasPrefix(got, "file://") {
		t.Errorf("URL without base = %q, want file:// prefix", got)
	}
}
