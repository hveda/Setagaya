package ports

import (
	"context"
	"errors"
	"time"
)

// ErrEnginesUnreachable is returned when a plan's engines are not yet routable
// (e.g. ingress not provisioned). Callers may retry.
var ErrEnginesUnreachable = errors.New("ports: engines unreachable")

// DeploySpec describes the engines to run for a single plan of a collection.
type DeploySpec struct {
	ProjectID   int64
	ExecutionID int64
	PlanID      int64
	Engines     int
	Image       string // executor container image
	CPU         string // optional resource request/limit, e.g. "1"
	Memory      string // optional resource request/limit, e.g. "512Mi"
}

// PlanRef names a plan and how many engines it expects; used to query status.
type PlanRef struct {
	PlanID  int64
	Engines int
}

// PlanReadiness reports how many of a plan's engines are up and whether they
// are reachable for triggering.
type PlanReadiness struct {
	PlanID          int64 `json:"plan_id"`
	EnginesWanted   int   `json:"engines"`
	EnginesDeployed int   `json:"engines_deployed"`
	Reachable       bool  `json:"engines_reachable"`
}

// CollectionStatus aggregates plan readiness for a collection.
type CollectionStatus struct {
	Plans    []PlanReadiness `json:"status"`
	PoolSize int             `json:"pool_size"`
}

// EngineDetail describes one engine pod.
type EngineDetail struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	CreatedTime time.Time `json:"created_time"`
}

// CollectionDetail describes a collection's ingress and engine pods.
type CollectionDetail struct {
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
// collection's engines in a cluster. It is orchestration only: what a test
// tool does on an engine is the Executor's concern.
type Scheduler interface {
	// DeployPlan ensures Engines replicas exist for the plan in spec. It is
	// idempotent: deploying an already-deployed plan is a no-op.
	DeployPlan(ctx context.Context, spec DeploySpec) error
	// EngineURLs returns the reachable base URLs of a plan's engines, ordered
	// by engine id (index 0..engines-1). Returns ErrEnginesUnreachable if the
	// engines are not yet routable.
	EngineURLs(ctx context.Context, executionID, planID int64, engines int) ([]string, error)
	// CollectionStatus reports per-plan readiness for the given plans.
	CollectionStatus(ctx context.Context, executionID int64, plans []PlanRef) (CollectionStatus, error)
	// EngineDetail reports the ingress IP and engine pods of a collection.
	EngineDetail(ctx context.Context, projectID, executionID int64) (CollectionDetail, error)
	// PurgeCollection removes all engines, services, and ingress of a
	// collection. Purging a collection with nothing deployed is not an error.
	PurgeCollection(ctx context.Context, executionID int64) error
	// PodLog returns the current logs of a plan's first engine pod.
	PodLog(ctx context.Context, executionID, planID int64) (string, error)
	// DeployedCollections maps deployed collection id to its earliest deploy
	// time; used by the auto-purge garbage collector.
	DeployedCollections(ctx context.Context) (map[int64]time.Time, error)
	// NodePools summarises the cluster node pools backing engine capacity.
	NodePools(ctx context.Context) ([]NodePool, error)
}
