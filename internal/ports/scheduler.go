package ports

import (
	"context"
	"errors"
	"time"
)

// ErrEnginesUnreachable is returned when a scenario's engines are not yet routable
// (e.g. ingress not provisioned). Callers may retry.
var ErrEnginesUnreachable = errors.New("ports: engines unreachable")

// DeploySpec describes the engines to run for a single scenario of an execution.
type DeploySpec struct {
	ProjectID   int64
	ExecutionID int64
	ScenarioID  int64
	Engines     int
	Image       string // executor container image
	CPU         string // optional resource request/limit, e.g. "1"
	Memory      string // optional resource request/limit, e.g. "512Mi"
}

// ScenarioRef names a scenario and how many engines it expects; used to query status.
type ScenarioRef struct {
	ScenarioID int64
	Engines    int
}

// ScenarioReadiness reports how many of a scenario's engines are up and whether they
// are reachable for triggering.
type ScenarioReadiness struct {
	ScenarioID      int64 `json:"scenario_id"`
	EnginesWanted   int   `json:"engines"`
	EnginesDeployed int   `json:"engines_deployed"`
	Reachable       bool  `json:"engines_reachable"`
}

// ExecutionStatus aggregates scenario readiness for an execution.
type ExecutionStatus struct {
	Scenarios []ScenarioReadiness `json:"status"`
	PoolSize  int                 `json:"pool_size"`
}

// EngineDetail describes one engine pod.
type EngineDetail struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	CreatedTime time.Time `json:"created_time"`
}

// ExecutionDetail describes an execution's ingress and engine pods.
type ExecutionDetail struct {
	IngressIP string         `json:"ingress_ip"`
	Engines   []EngineDetail `json:"engines"`
}

// NodePool summarises a cluster node pool backing engine capacity.
type NodePool struct {
	Name       string    `json:"name"`
	Size       int       `json:"size"`
	LaunchTime time.Time `json:"launch_time"`
}

// Scheduler manages the compute (pods, services, ingress) backing a
// execution's engines in a cluster. It is orchestration only: what a test
// tool does on an engine is the Executor's concern.
type Scheduler interface {
	// DeployScenario ensures Engines replicas exist for the scenario in spec. It is
	// idempotent: deploying an already-deployed scenario is a no-op.
	DeployScenario(ctx context.Context, spec DeploySpec) error
	// EngineURLs returns the reachable base URLs of a scenario's engines, ordered
	// by engine id (index 0..engines-1). Returns ErrEnginesUnreachable if the
	// engines are not yet routable.
	EngineURLs(ctx context.Context, executionID, scenarioID int64, engines int) ([]string, error)
	// ExecutionStatus reports per-scenario readiness for the given scenarios.
	ExecutionStatus(ctx context.Context, executionID int64, scenarios []ScenarioRef) (ExecutionStatus, error)
	// EngineDetail reports the ingress IP and engine pods of an execution.
	EngineDetail(ctx context.Context, projectID, executionID int64) (ExecutionDetail, error)
	// PurgeExecution removes all engines, services, and ingress of a
	// execution. Purging an execution with nothing deployed is not an error.
	PurgeExecution(ctx context.Context, executionID int64) error
	// PodLog returns the current logs of a scenario's first engine pod.
	PodLog(ctx context.Context, executionID, scenarioID int64) (string, error)
	// DeployedExecutions maps deployed execution id to its earliest deploy
	// time; used by the auto-purge garbage collector.
	DeployedExecutions(ctx context.Context) (map[int64]time.Time, error)
	// NodePools summarises the cluster node pools backing engine capacity.
	NodePools(ctx context.Context) ([]NodePool, error)
}
