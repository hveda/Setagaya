package ports

import (
	"context"
	"errors"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
)

// ErrClusterExists is returned by CreateCluster when an entry with the same
// name already exists. Callers compare with errors.Is.
var ErrClusterExists = errors.New("ports: cluster already exists")

// ClusterRegistry persists registered clusters (clusterregistry.Cluster),
// keyed by name, and resolves a ClusterRef to its entry for the scheduler.
type ClusterRegistry interface {
	// CreateCluster stores c, returning ErrClusterExists if an entry named
	// c.Name already exists.
	CreateCluster(ctx context.Context, c clusterregistry.Cluster) error
	// GetCluster returns the cluster named name, or ErrNotFound.
	GetCluster(ctx context.Context, name string) (clusterregistry.Cluster, error)
	// ListClusters returns every registered cluster, ordered by name.
	ListClusters(ctx context.Context) ([]clusterregistry.Cluster, error)
	// UpdateCluster replaces the entry named c.Name, or ErrNotFound if none
	// exists. Created metadata (created_by, created_time) is immutable after
	// registration and is preserved regardless of what c carries.
	UpdateCluster(ctx context.Context, c clusterregistry.Cluster) error
	// DeleteCluster removes the cluster named name, or ErrNotFound.
	DeleteCluster(ctx context.Context, name string) error
	// ResolveCluster returns the entry a ClusterRef names, or ErrNotFound for
	// an unknown ref. The scheduler resolves the implicit default
	// (clusterregistry.IsDefaultName) to rest.InClusterConfig() directly,
	// before ever calling this -- an empty ref reaches here only as a lookup
	// miss.
	ResolveCluster(ctx context.Context, ref ClusterRef) (clusterregistry.Cluster, error)

	// SetClusterCredential stores a BYOC cluster's credential against an
	// existing entry, or ErrNotFound. The bytes are opaque and stored
	// verbatim; envelope encryption (secretbox) is applied above this port, so
	// only ciphertext is ever persisted -- the store and the plaintext never
	// meet. Passing nil clears the stored credential.
	SetClusterCredential(ctx context.Context, name string, ciphertext []byte) error
	// SetClusterIngestTokenHash stores SHA-256 of a cluster's ingest token
	// against an existing entry, or ErrNotFound. Overwriting is rotation: the
	// previous token dies with the write. An empty hash clears it. The hash
	// is derived data (see clusterregistry.HashToken), never a credential.
	SetClusterIngestTokenHash(ctx context.Context, name string, hash string) error
	// ClusterByIngestTokenHash resolves an ingest token's hash to its cluster,
	// or ErrNotFound when no entry carries it. The ingest endpoint
	// authenticates a presented token by hashing it and calling this.
	ClusterByIngestTokenHash(ctx context.Context, hash string) (clusterregistry.Cluster, error)
	// GetClusterCredential returns the stored ciphertext for name, ErrNotFound
	// if the entry is absent, or nil for an entry with no credential (an
	// operator-managed cluster).
	GetClusterCredential(ctx context.Context, name string) ([]byte, error)
}
