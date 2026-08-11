package fake

import (
	"context"
	"sort"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/ports"
)

// CreateCluster stores c, or ports.ErrClusterExists if c.Name is taken.
func (s *Store) CreateCluster(_ context.Context, c clusterregistry.Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clusters[c.Name]; ok {
		return ports.ErrClusterExists
	}
	s.clusters[c.Name] = c
	return nil
}

// GetCluster returns the cluster named name, or ports.ErrNotFound.
func (s *Store) GetCluster(_ context.Context, name string) (clusterregistry.Cluster, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.clusters[name]
	if !ok {
		return clusterregistry.Cluster{}, ports.ErrNotFound
	}
	return c, nil
}

// ListClusters returns every registered cluster, ordered by name.
func (s *Store) ListClusters(_ context.Context) ([]clusterregistry.Cluster, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]clusterregistry.Cluster, 0, len(s.clusters))
	for _, c := range s.clusters {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// UpdateCluster replaces the entry named c.Name, preserving its immutable
// created metadata, or returns ports.ErrNotFound.
func (s *Store) UpdateCluster(_ context.Context, c clusterregistry.Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.clusters[c.Name]
	if !ok {
		return ports.ErrNotFound
	}
	// Created metadata is set once at registration; an update never rewrites
	// it, matching the mysql UPDATE that omits those columns.
	c.CreatedBy = existing.CreatedBy
	c.CreatedTime = existing.CreatedTime
	s.clusters[c.Name] = c
	return nil
}

// DeleteCluster removes the cluster named name, or ports.ErrNotFound.
func (s *Store) DeleteCluster(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clusters[name]; !ok {
		return ports.ErrNotFound
	}
	delete(s.clusters, name)
	return nil
}

// ResolveCluster returns the entry ref names, or ports.ErrNotFound.
func (s *Store) ResolveCluster(ctx context.Context, ref ports.ClusterRef) (clusterregistry.Cluster, error) {
	return s.GetCluster(ctx, string(ref))
}

// SetClusterCredential stores name's credential ciphertext verbatim, or
// ports.ErrNotFound if the entry is absent. nil clears it.
func (s *Store) SetClusterCredential(_ context.Context, name string, ciphertext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clusters[name]; !ok {
		return ports.ErrNotFound
	}
	if ciphertext == nil {
		delete(s.clusterCredentials, name)
		return nil
	}
	stored := make([]byte, len(ciphertext))
	copy(stored, ciphertext)
	s.clusterCredentials[name] = stored
	return nil
}

// GetClusterCredential returns name's stored ciphertext, ports.ErrNotFound if
// the entry is absent, or nil for an entry with no credential.
func (s *Store) GetClusterCredential(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clusters[name]; !ok {
		return nil, ports.ErrNotFound
	}
	ct, ok := s.clusterCredentials[name]
	if !ok {
		return nil, nil
	}
	out := make([]byte, len(ct))
	copy(out, ct)
	return out, nil
}
