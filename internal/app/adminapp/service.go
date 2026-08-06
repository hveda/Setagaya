// Package adminapp is the operations use-case: it lists the executions
// currently holding engines, reports cluster node pools, and auto-purges engines
// left idle past a threshold.
package adminapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/ports"
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
	ExecutionID int64     `json:"execution_id"`
	Name        string    `json:"name"`
	ProjectID   int64     `json:"project_id"`
	DeployedAt  time.Time `json:"deployed_at"`
	Running     bool      `json:"running"`
}

// RunningExecutions lists every deployed execution, enriched with its name,
// project, and whether a run is in progress.
func (s *Service) RunningExecutions(ctx context.Context) ([]RunningExecution, error) {
	deployed, err := s.sched.DeployedExecutions(ctx, "")
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
	return s.sched.NodePools(ctx, "")
}

// AutoPurgeStale purges every execution whose engines have been deployed longer
// than idleFor and which has no run in progress. It returns the purged ids.
func (s *Service) AutoPurgeStale(ctx context.Context, idleFor time.Duration) ([]int64, error) {
	deployed, err := s.sched.DeployedExecutions(ctx, "")
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

// abortGracePeriod bounds how long one Abort call may take tearing down
// every matching execution, mirroring the k8s scheduler adapter's own
// default pod termination grace: admin can't reasonably wait longer than a
// single pod's own shutdown budget, however many it's tearing down at once.
const abortGracePeriod = 30 * time.Second

// Scope selects which in-flight (deployed) executions a kill-switch
// invocation targets.
type Scope string

const (
	// ScopeTenant matches every deployed execution belonging to a tenant.
	// value is the tenant id.
	ScopeTenant Scope = "tenant"
	// ScopeCluster matches every deployed execution on a cluster. value is
	// the cluster ref (empty string is this deployment's own default
	// cluster, the only one there is until Phase 8).
	ScopeCluster Scope = "cluster"
	// ScopeCampaign is a valid scope now so this endpoint doesn't need
	// touching again once Phase 6 introduces campaigns -- unreachable until
	// then, so it always matches nothing rather than erroring.
	ScopeCampaign Scope = "campaign"
	// ScopeExecutionList matches exactly the given executions. value is a
	// comma-separated list of execution ids.
	ScopeExecutionList Scope = "execution_list"
)

// ErrScopeInvalid is returned for an unknown scope, or a value that cannot
// be parsed for the scope given.
var ErrScopeInvalid = errors.New("adminapp: invalid kill-switch scope or value")

// Abort tears down every in-flight execution matching scope+value, via the
// existing Purge (which itself stops any run in progress first), within a
// bounded time. One execution failing to purge does not stop the rest --
// the returned ids are exactly the ones actually torn down, so a partial
// abort is visible rather than silently reported as complete.
func (s *Service) Abort(ctx context.Context, scope Scope, value string) ([]int64, error) {
	targets, err := s.matchingExecutions(ctx, scope, value)
	if err != nil {
		return nil, err
	}
	boundedCtx, cancel := context.WithTimeout(ctx, abortGracePeriod)
	defer cancel()

	var aborted []int64
	for _, executionID := range targets {
		if err := s.purger.Purge(boundedCtx, executionID); err != nil {
			continue
		}
		aborted = append(aborted, executionID)
	}
	sort.Slice(aborted, func(i, j int) bool { return aborted[i] < aborted[j] })
	return aborted, nil
}

// matchingExecutions resolves scope+value into the deployed execution ids it
// selects.
func (s *Service) matchingExecutions(ctx context.Context, scope Scope, value string) ([]int64, error) {
	switch scope {
	case ScopeExecutionList:
		return parseExecutionList(value)
	case ScopeCampaign:
		return nil, nil
	case ScopeTenant:
		tenantID, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid tenant id %q", ErrScopeInvalid, value)
		}
		return s.deployedExecutionsForTenant(ctx, tenantID)
	case ScopeCluster:
		return s.deployedExecutionIDs(ctx, ports.ClusterRef(value))
	default:
		return nil, fmt.Errorf("%w: %q", ErrScopeInvalid, scope)
	}
}

func (s *Service) deployedExecutionIDs(ctx context.Context, cluster ports.ClusterRef) ([]int64, error) {
	deployed, err := s.sched.DeployedExecutions(ctx, cluster)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(deployed))
	for executionID := range deployed {
		out = append(out, executionID)
	}
	return out, nil
}

func (s *Service) deployedExecutionsForTenant(ctx context.Context, tenantID int64) ([]int64, error) {
	ids, err := s.deployedExecutionIDs(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, executionID := range ids {
		exe, err := s.repo.GetExecution(ctx, executionID)
		if err != nil {
			continue // a deployed execution whose record vanished has nothing left to match against
		}
		if exe.TenantID != nil && *exe.TenantID == tenantID {
			out = append(out, executionID)
		}
	}
	return out, nil
}

func parseExecutionList(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid execution id %q", ErrScopeInvalid, p)
		}
		out = append(out, id)
	}
	return out, nil
}
