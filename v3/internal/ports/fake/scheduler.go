package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Scheduler is an in-memory ports.Scheduler for fast use-case tests. It records
// deployments and answers status/URL queries from that record. Reachability and
// pod logs are controllable via exported fields.
type Scheduler struct {
	mu          sync.Mutex
	deployments map[int64]map[int64]schedDeploy // collection -> plan -> deploy

	// Unreachable, when true, makes engines appear deployed but not routable.
	Unreachable bool
	// PodLogText is returned by PodLog for deployed plans.
	PodLogText string
	// IngressIP is reported by EngineDetail.
	IngressIP string
	// Pools is returned by NodePools.
	Pools []ports.NodePool
	// Now supplies deploy timestamps; defaults to time.Now.
	Now func() time.Time
}

type schedDeploy struct {
	spec     ports.DeploySpec
	deployAt time.Time
}

// NewScheduler returns an empty in-memory Scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		deployments: map[int64]map[int64]schedDeploy{},
		PodLogText:  "fake engine log",
		IngressIP:   "10.0.0.1",
		Pools:       []ports.NodePool{{Name: "default", Size: 1}},
	}
}

// NodePools returns the configured pools.
func (s *Scheduler) NodePools(_ context.Context) ([]ports.NodePool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ports.NodePool(nil), s.Pools...), nil
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// DeployPlan records the deployment, keeping the earliest deploy time per plan.
func (s *Scheduler) DeployPlan(_ context.Context, spec ports.DeploySpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plans, ok := s.deployments[spec.CollectionID]
	if !ok {
		plans = map[int64]schedDeploy{}
		s.deployments[spec.CollectionID] = plans
	}
	if existing, ok := plans[spec.PlanID]; ok {
		existing.spec = spec
		plans[spec.PlanID] = existing // keep original deployAt (idempotent)
		return nil
	}
	plans[spec.PlanID] = schedDeploy{spec: spec, deployAt: s.now()}
	return nil
}

// EngineURLs returns synthetic per-engine URLs, or ErrEnginesUnreachable.
func (s *Scheduler) EngineURLs(_ context.Context, collectionID, planID int64, engines int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deployments[collectionID][planID]
	if !ok || s.Unreachable {
		return nil, ports.ErrEnginesUnreachable
	}
	urls := make([]string, engines)
	for i := range urls {
		urls[i] = fmt.Sprintf("http://engine-%d-%d-%d-%d.fake", d.spec.ProjectID, collectionID, planID, i)
	}
	return urls, nil
}

// CollectionStatus reports deployed/wanted engines and reachability per plan.
func (s *Scheduler) CollectionStatus(_ context.Context, collectionID int64, plans []ports.PlanRef) (ports.CollectionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := ports.CollectionStatus{}
	for _, ref := range plans {
		pr := ports.PlanReadiness{PlanID: ref.PlanID, EnginesWanted: ref.Engines}
		if d, ok := s.deployments[collectionID][ref.PlanID]; ok {
			pr.EnginesDeployed = d.spec.Engines
			pr.Reachable = !s.Unreachable
		}
		status.PoolSize += pr.EnginesDeployed
		status.Plans = append(status.Plans, pr)
	}
	return status, nil
}

// EngineDetail lists the engine pods of a collection.
func (s *Scheduler) EngineDetail(_ context.Context, _, collectionID int64) (ports.CollectionDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	detail := ports.CollectionDetail{IngressIP: s.IngressIP}
	planIDs := make([]int64, 0, len(s.deployments[collectionID]))
	for planID := range s.deployments[collectionID] {
		planIDs = append(planIDs, planID)
	}
	sort.Slice(planIDs, func(i, j int) bool { return planIDs[i] < planIDs[j] })
	for _, planID := range planIDs {
		d := s.deployments[collectionID][planID]
		for i := 0; i < d.spec.Engines; i++ {
			detail.Engines = append(detail.Engines, ports.EngineDetail{
				Name:        fmt.Sprintf("engine-%d-%d-%d-%d", d.spec.ProjectID, collectionID, planID, i),
				Status:      "Running",
				CreatedTime: d.deployAt,
			})
		}
	}
	return detail, nil
}

// PurgeCollection removes all record of a collection's deployments.
func (s *Scheduler) PurgeCollection(_ context.Context, collectionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.deployments, collectionID)
	return nil
}

// PodLog returns the canned log for a deployed plan.
func (s *Scheduler) PodLog(_ context.Context, collectionID, planID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.deployments[collectionID][planID]; !ok {
		return "", ports.ErrEnginesUnreachable
	}
	return s.PodLogText, nil
}

// DeployedCollections maps collection id to its earliest deploy time.
func (s *Scheduler) DeployedCollections(_ context.Context) (map[int64]time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[int64]time.Time{}
	for collectionID, plans := range s.deployments {
		for _, d := range plans {
			if cur, ok := out[collectionID]; !ok || d.deployAt.Before(cur) {
				out[collectionID] = d.deployAt
			}
		}
	}
	return out, nil
}
