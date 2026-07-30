// Package adminapp is the operations use-case: it lists the executions
// currently holding engines, reports cluster node pools, and auto-purges engines
// left idle past a threshold.
package adminapp

import (
	"context"
	"sort"
	"time"

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Repo is the persistence admin needs to enrich and evaluate executions.
type Repo interface {
	GetExecution(ctx context.Context, id int64) (execution.Execution, error)
	CurrentRun(ctx context.Context, executionID int64) (int64, bool, error)
}

// Purger tears down a execution's engines. The lifecycle service satisfies it.
type Purger interface {
	Purge(ctx context.Context, executionID int64) error
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

// RunningExecution describes an execution currently holding engines.
type RunningExecution struct {
	ExecutionID int64     `json:"collection_id"`
	Name        string    `json:"name"`
	ProjectID   int64     `json:"project_id"`
	DeployedAt  time.Time `json:"deployed_at"`
	Running     bool      `json:"running"`
}

// RunningExecutions lists every deployed execution, enriched with its name,
// project, and whether a run is in progress.
func (s *Service) RunningExecutions(ctx context.Context) ([]RunningExecution, error) {
	deployed, err := s.sched.DeployedExecutions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RunningExecution, 0, len(deployed))
	for executionID, deployedAt := range deployed {
		rc := RunningExecution{ExecutionID: executionID, DeployedAt: deployedAt}
		if c, err := s.repo.GetExecution(ctx, executionID); err == nil {
			rc.Name = c.Name
			rc.ProjectID = c.ProjectID
		}
		if _, running, err := s.repo.CurrentRun(ctx, executionID); err == nil {
			rc.Running = running
		}
		out = append(out, rc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExecutionID < out[j].ExecutionID })
	return out, nil
}

// NodePools reports the cluster node pools.
func (s *Service) NodePools(ctx context.Context) ([]ports.NodePool, error) {
	return s.sched.NodePools(ctx)
}

// AutoPurgeStale purges every execution whose engines have been deployed longer
// than idleFor and which has no run in progress. It returns the purged ids.
func (s *Service) AutoPurgeStale(ctx context.Context, idleFor time.Duration) ([]int64, error) {
	deployed, err := s.sched.DeployedExecutions(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var purged []int64
	for executionID, deployedAt := range deployed {
		if now.Sub(deployedAt) < idleFor {
			continue
		}
		if _, running, err := s.repo.CurrentRun(ctx, executionID); err != nil || running {
			continue
		}
		if err := s.purger.Purge(ctx, executionID); err != nil {
			continue
		}
		purged = append(purged, executionID)
	}
	sort.Slice(purged, func(i, j int) bool { return purged[i] < purged[j] })
	return purged, nil
}
