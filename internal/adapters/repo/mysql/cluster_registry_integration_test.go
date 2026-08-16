//go:build integration

package mysql_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLClusterRegistry_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunClusterRegistryContract(t, func(t *testing.T) ports.ClusterRegistry {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// The credential BLOB is opaque ciphertext stored verbatim -- no encryption
// here (task 89). An operator entry has none (NULL -> nil).
func TestMySQLClusterRegistry_CredentialBlobRoundTrips(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	entry := clusterregistry.Cluster{
		Name: "prod-eu", APIURL: "https://prod-eu:6443", CACert: "ca", IngestURL: "http://ingest",
		SidecarImage: "img", Namespace: "honryu", SecretRef: "prod-eu-creds",
		Origin: clusterregistry.OriginBYOC, CreatedBy: "admin", CreatedTime: time.Now().UTC(),
	}
	if err := repo.CreateCluster(ctx, entry); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	// No credential yet: a fresh row's blob is NULL.
	got, err := repo.GetClusterCredential(ctx, "prod-eu")
	if err != nil {
		t.Fatalf("GetClusterCredential (empty): %v", err)
	}
	if got != nil {
		t.Fatalf("GetClusterCredential (empty) = %v, want nil", got)
	}

	// Arbitrary bytes, including a NUL, must survive verbatim.
	cipher := []byte{0x00, 0x01, 0xff, 0x7f, 0x80, 'k', 'u', 'b', 'e'}
	if err := repo.SetClusterCredential(ctx, "prod-eu", cipher); err != nil {
		t.Fatalf("SetClusterCredential: %v", err)
	}
	got, err = repo.GetClusterCredential(ctx, "prod-eu")
	if err != nil {
		t.Fatalf("GetClusterCredential: %v", err)
	}
	if !bytes.Equal(got, cipher) {
		t.Fatalf("GetClusterCredential = %v, want %v", got, cipher)
	}

	// A CRUD update must not disturb the stored credential.
	entry.IngestURL = "http://ingest-2"
	if err := repo.UpdateCluster(ctx, entry); err != nil {
		t.Fatalf("UpdateCluster: %v", err)
	}
	got, err = repo.GetClusterCredential(ctx, "prod-eu")
	if err != nil {
		t.Fatalf("GetClusterCredential (after update): %v", err)
	}
	if !bytes.Equal(got, cipher) {
		t.Fatalf("credential disturbed by UpdateCluster: got %v, want %v", got, cipher)
	}

	// Unknown entry is not-found for both accessors.
	if err := repo.SetClusterCredential(ctx, "ghost", cipher); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("SetClusterCredential(ghost) = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetClusterCredential(ctx, "ghost"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("GetClusterCredential(ghost) = %v, want ErrNotFound", err)
	}
}
