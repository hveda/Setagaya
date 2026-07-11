// Package adminapp is the operations use-case: it lists the collections
// currently holding engines, reports cluster node pools, and auto-purges engines
// left idle past a threshold.
package adminapp

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Repo is the persistence admin needs to enrich and evaluate collections.
type Repo interface {
	GetCollection(ctx context.Context, id int64) (collection.Collection, error)
	CurrentRun(ctx context.Context, collectionID int64) (int64, bool, error)
}

// Purger tears down a collection's engines. The lifecycle service satisfies it.
type Purger interface {
	Purge(ctx context.Context, collectionID int64) error
}

// Service implements the admin use-cases.
type Service struct {
	repo   Repo
	sched  ports.Scheduler
	purger Purger
	now    func() time.Time
}

// NewService wires the admin service.
func NewService(repo Repo, sched ports.Scheduler, purger Purger) *Service {
	return &Service{repo: repo, sched: sched, purger: purger, now: time.Now}
}

// RunningCollection describes a collection currently holding engines.
type RunningCollection struct {
	CollectionID int64     `json:"collection_id"`
	Name         string    `json:"name"`
	ProjectID    int64     `json:"project_id"`
	DeployedAt   time.Time `json:"deployed_at"`
	Running      bool      `json:"running"`
}

// RunningCollections lists every deployed collection, enriched with its name,
// project, and whether a run is in progress.
func (s *Service) RunningCollections(ctx context.Context) ([]RunningCollection, error) {
	deployed, err := s.sched.DeployedCollections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RunningCollection, 0, len(deployed))
	for collectionID, deployedAt := range deployed {
		rc := RunningCollection{CollectionID: collectionID, DeployedAt: deployedAt}
		if c, err := s.repo.GetCollection(ctx, collectionID); err == nil {
			rc.Name = c.Name
			rc.ProjectID = c.ProjectID
		}
		if _, running, err := s.repo.CurrentRun(ctx, collectionID); err == nil {
			rc.Running = running
		}
		out = append(out, rc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CollectionID < out[j].CollectionID })
	return out, nil
}

// NodePools reports the cluster node pools.
func (s *Service) NodePools(ctx context.Context) ([]ports.NodePool, error) {
	return s.sched.NodePools(ctx)
}

// AutoPurgeStale purges every collection whose engines have been deployed longer
// than idleFor and which has no run in progress. It returns the purged ids.
func (s *Service) AutoPurgeStale(ctx context.Context, idleFor time.Duration) ([]int64, error) {
	deployed, err := s.sched.DeployedCollections(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var purged []int64
	for collectionID, deployedAt := range deployed {
		if now.Sub(deployedAt) < idleFor {
			continue
		}
		if _, running, err := s.repo.CurrentRun(ctx, collectionID); err != nil || running {
			continue
		}
		if err := s.purger.Purge(ctx, collectionID); err != nil {
			continue
		}
		purged = append(purged, collectionID)
	}
	sort.Slice(purged, func(i, j int) bool { return purged[i] < purged[j] })
	return purged, nil
}
