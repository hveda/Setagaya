package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/heridotlife/honryu/internal/ports"
)

// Scheduler is an in-memory ports.Scheduler for fast use-case tests. It records
// deployments and answers status/URL queries from that record. Reachability and
// pod logs are controllable via exported fields.
type Scheduler struct {
	mu          sync.Mutex
	deployments map[int64]map[int64]schedDeploy // execution -> scenario -> deploy

	// Unreachable, when true, makes engines appear deployed but not routable.
	Unreachable bool
	// PodLogText is returned by PodLog for deployed scenarios.
	PodLogText string
	// PodLogErr, when set, is returned by PodLog instead of a log.
	PodLogErr error
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
func (s *Scheduler) NodePools(_ context.Context, _ ports.ClusterRef) ([]ports.NodePool, error) {
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

// DeployScenario records the deployment, keeping the earliest deploy time per scenario.
func (s *Scheduler) DeployScenario(_ context.Context, spec ports.DeploySpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	scenarios, ok := s.deployments[spec.ExecutionID]
	if !ok {
		scenarios = map[int64]schedDeploy{}
		s.deployments[spec.ExecutionID] = scenarios
	}
	if existing, ok := scenarios[spec.ScenarioID]; ok {
		existing.spec = spec
		scenarios[spec.ScenarioID] = existing // keep original deployAt (idempotent)
		return nil
	}
	scenarios[spec.ScenarioID] = schedDeploy{spec: spec, deployAt: s.now()}
	return nil
}

// ExecutionStatus reports deployed/wanted engines and reachability per scenario.
// LastDeploy returns the spec a scenario was last deployed with, so a test can
// assert what each pod was actually given rather than only that a deploy
// happened.
func (s *Scheduler) LastDeploy(executionID, scenarioID int64) (ports.DeploySpec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deployments[executionID][scenarioID]
	if !ok {
		return ports.DeploySpec{}, false
	}
	return d.spec, true
}

func (s *Scheduler) ExecutionStatus(_ context.Context, _ ports.ClusterRef, executionID int64, scenarios []ports.ScenarioRef) (ports.ExecutionStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := ports.ExecutionStatus{}
	for _, ref := range scenarios {
		pr := ports.ScenarioReadiness{ScenarioID: ref.ScenarioID, EnginesWanted: ref.Shards}
		if d, ok := s.deployments[executionID][ref.ScenarioID]; ok {
			pr.EnginesDeployed = len(d.spec.Shards)
			pr.Reachable = !s.Unreachable
		}
		status.PoolSize += pr.EnginesDeployed
		status.Scenarios = append(status.Scenarios, pr)
	}
	return status, nil
}

// EngineDetail lists the engine pods of an execution.
func (s *Scheduler) EngineDetail(_ context.Context, _ ports.ClusterRef, _, executionID int64) (ports.ExecutionDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	detail := ports.ExecutionDetail{IngressIP: s.IngressIP}
	planIDs := make([]int64, 0, len(s.deployments[executionID]))
	for scenarioID := range s.deployments[executionID] {
		planIDs = append(planIDs, scenarioID)
	}
	sort.Slice(planIDs, func(i, j int) bool { return planIDs[i] < planIDs[j] })
	for _, scenarioID := range planIDs {
		d := s.deployments[executionID][scenarioID]
		for i := 0; i < len(d.spec.Shards); i++ {
			detail.Engines = append(detail.Engines, ports.EngineDetail{
				Name:        fmt.Sprintf("engine-%d-%d-%d-%d", d.spec.ProjectID, executionID, scenarioID, i),
				Status:      "Running",
				CreatedTime: d.deployAt,
			})
		}
	}
	return detail, nil
}

// PurgeExecution removes all record of a execution's deployments.
func (s *Scheduler) PurgeExecution(_ context.Context, _ ports.ClusterRef, executionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.deployments, executionID)
	return nil
}

// PodLog returns the canned log for a deployed scenario.
func (s *Scheduler) PodLog(_ context.Context, _ ports.ClusterRef, executionID, scenarioID int64, shard int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.deployments[executionID][scenarioID]
	if !ok || shard < 0 || shard >= len(d.spec.Shards) {
		return "", ports.ErrEnginesUnreachable
	}
	if s.PodLogErr != nil {
		return "", s.PodLogErr
	}
	// Shard-specific, like a real pod's: the port promises logs addressed per
	// pod, and a fake that answered every shard identically could not catch a
	// caller that silently ignored the one it asked for.
	return fmt.Sprintf("%s (shard %d)", s.PodLogText, shard), nil
}

// DeployedExecutions maps execution id to its earliest deploy time.
func (s *Scheduler) DeployedExecutions(_ context.Context, _ ports.ClusterRef) (map[int64]time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[int64]time.Time{}
	for executionID, scenarios := range s.deployments {
		for _, d := range scenarios {
			if cur, ok := out[executionID]; !ok || d.deployAt.Before(cur) {
				out[executionID] = d.deployAt
			}
		}
	}
	return out, nil
}
