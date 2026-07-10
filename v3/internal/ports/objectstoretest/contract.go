// Package objectstoretest provides a reusable conformance suite for the
// ObjectStore port, run against both the fake and the filesystem adapter.
package objectstoretest

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/hveda/Setagaya/v3/internal/ports"
)

// NewStore returns a fresh, empty ObjectStore for a single subtest.
type NewStore func(t *testing.T) ports.ObjectStore

// RunObjectStoreContract exercises the behaviour every ObjectStore must satisfy.
func RunObjectStoreContract(t *testing.T, newStore NewStore) {
	t.Helper()

	t.Run("UploadDownloadRoundTrip", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		want := []byte("id,name\n1,alice\n")

		if err := store.Upload(ctx, "plan/42/users.csv", bytes.NewReader(want)); err != nil {
			t.Fatalf("Upload: %v", err)
		}
		got, err := store.Download(ctx, "plan/42/users.csv")
		if err != nil {
			t.Fatalf("Download: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("Download = %q, want %q", got, want)
		}
	})

	t.Run("DownloadMissingReturnsNotFound", func(t *testing.T) {
		store := newStore(t)
		if _, err := store.Download(context.Background(), "plan/1/nope.jmx"); !errors.Is(err, ports.ErrObjectNotFound) {
			t.Fatalf("Download(missing) = %v, want ErrObjectNotFound", err)
		}
	})

	t.Run("UploadOverwrites", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		key := "collection/7/config.yaml"

		if err := store.Upload(ctx, key, bytes.NewReader([]byte("v1"))); err != nil {
			t.Fatalf("Upload v1: %v", err)
		}
		if err := store.Upload(ctx, key, bytes.NewReader([]byte("v2"))); err != nil {
			t.Fatalf("Upload v2: %v", err)
		}
		got, err := store.Download(ctx, key)
		if err != nil {
			t.Fatalf("Download: %v", err)
		}
		if string(got) != "v2" {
			t.Fatalf("Download = %q, want v2", got)
		}
	})

	t.Run("DeleteRemovesAndIsIdempotent", func(t *testing.T) {
		store := newStore(t)
		ctx := context.Background()
		key := "plan/9/test.jmx"

		if err := store.Upload(ctx, key, bytes.NewReader([]byte("x"))); err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if err := store.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := store.Download(ctx, key); !errors.Is(err, ports.ErrObjectNotFound) {
			t.Fatalf("Download after delete = %v, want ErrObjectNotFound", err)
		}
		// Deleting a missing key is not an error.
		if err := store.Delete(ctx, key); err != nil {
			t.Fatalf("Delete(missing) = %v, want nil", err)
		}
	})
}
