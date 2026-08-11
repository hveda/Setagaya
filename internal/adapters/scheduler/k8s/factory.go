package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/ports"
)

// clientBuilder builds a clientset from a rest.Config; injectable for tests.
type clientBuilder func(*rest.Config) (kubernetes.Interface, error)

// DefaultDeploy is the default (empty-ref) cluster's deploy settings: the
// control plane's own cluster, which has no registry row and so carries these
// from config rather than an entry. namespace also names where the registry's
// credential Secrets live.
type DefaultDeploy struct {
	Namespace    string
	SidecarImage string
	IngestURL    string
}

// resolved is a cluster resolved to its API client and per-cluster deploy
// settings. For a registered cluster these come from its entry; for the default
// cluster, from DefaultDeploy.
type resolved struct {
	client       kubernetes.Interface
	namespace    string
	sidecarImage string
	ingestURL    string
}

// ClientFactory resolves a ClusterRef to a Kubernetes client and its deploy
// settings. The default (empty) ref is the control plane's own cluster -- the
// home client, built from rest.InClusterConfig(), with DefaultDeploy settings.
// A registered ref is built once from its home-cluster credential Secret --
// with its namespace/sidecar/ingest taken from the entry, not global config --
// and cached; Invalidate drops the cache entry so the next use rebuilds from
// the (possibly rotated) Secret.
type ClientFactory struct {
	registry ports.ClusterRegistry
	build    clientBuilder
	def      resolved // the default cluster (home client + DefaultDeploy)

	mu    sync.Mutex
	cache map[ports.ClusterRef]resolved
}

// NewClientFactory builds a factory. homeClient is the control plane's own
// client: it both serves the default ref and holds the registry's credential
// Secrets, which live in def.Namespace.
func NewClientFactory(homeClient kubernetes.Interface, def DefaultDeploy, registry ports.ClusterRegistry) *ClientFactory {
	return &ClientFactory{
		registry: registry,
		build:    func(cfg *rest.Config) (kubernetes.Interface, error) { return kubernetes.NewForConfig(cfg) },
		def: resolved{
			client: homeClient, namespace: def.Namespace,
			sidecarImage: def.SidecarImage, ingestURL: def.IngestURL,
		},
		cache: map[ports.ClusterRef]resolved{},
	}
}

// Resolve returns the client and deploy settings for ref, building and caching
// them on first use. The default ref resolves to the home client + DefaultDeploy.
func (f *ClientFactory) Resolve(ctx context.Context, ref ports.ClusterRef) (resolved, error) {
	if clusterregistry.IsDefaultName(string(ref)) {
		return f.def, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.cache[ref]; ok {
		return r, nil
	}
	entry, err := f.registry.ResolveCluster(ctx, ref)
	if err != nil {
		return resolved{}, fmt.Errorf("k8s: resolve cluster %q: %w", ref, err)
	}
	// The credential Secrets live in the home cluster's namespace (def.namespace).
	cfg, err := RestConfigFromSecret(ctx, f.def.client, f.def.namespace, entry.SecretRef)
	if err != nil {
		return resolved{}, fmt.Errorf("k8s: credential for cluster %q: %w", ref, err)
	}
	client, err := f.build(cfg)
	if err != nil {
		return resolved{}, fmt.Errorf("k8s: build client for cluster %q: %w", ref, err)
	}
	r := resolved{
		client: client, namespace: entry.Namespace,
		sidecarImage: entry.SidecarImage, ingestURL: entry.IngestURL,
	}
	f.cache[ref] = r
	return r, nil
}

// Client returns just the Kubernetes client for ref.
func (f *ClientFactory) Client(ctx context.Context, ref ports.ClusterRef) (kubernetes.Interface, error) {
	r, err := f.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return r.client, nil
}

// Invalidate drops ref's cached client so the next resolve rebuilds it from the
// current Secret -- the recovery path after a credential rotation.
func (f *ClientFactory) Invalidate(ref ports.ClusterRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cache, ref)
}

// Router is a ports.Scheduler that dispatches each operation to a per-cluster
// bound Scheduler resolved through a ClientFactory. On an authentication
// failure (a rotated credential) it invalidates the cached client and retries
// once.
type Router struct {
	factory *ClientFactory
	cfg     Config
}

// NewRouter builds a Router over factory; cfg supplies the namespace, ports, and
// sidecar/ingest settings each bound Scheduler uses.
func NewRouter(factory *ClientFactory, cfg Config) *Router {
	return &Router{factory: factory, cfg: cfg}
}

var _ ports.Scheduler = (*Router)(nil)

func (r *Router) bound(ctx context.Context, cluster ports.ClusterRef) (*Scheduler, error) {
	res, err := r.factory.Resolve(ctx, cluster)
	if err != nil {
		return nil, err
	}
	// Namespace, sidecar image, and ingest URL are per-cluster (from the
	// resolved entry, or DefaultDeploy for the default); the engine port, pool
	// label, and termination grace are deployment-wide and come from r.cfg.
	return New(res.client, Config{
		Namespace:        res.namespace,
		SidecarImage:     res.sidecarImage,
		IngestURL:        res.ingestURL,
		EnginePort:       r.cfg.EnginePort,
		PoolLabel:        r.cfg.PoolLabel,
		TerminationGrace: r.cfg.TerminationGrace,
	}), nil
}

// routed resolves the bound scheduler for cluster, runs fn, and -- on an
// authentication failure -- rebuilds the client once and retries, which
// recovers from a rotated credential without a restart.
func routed[T any](ctx context.Context, r *Router, cluster ports.ClusterRef, fn func(*Scheduler) (T, error)) (T, error) {
	var zero T
	s, err := r.bound(ctx, cluster)
	if err != nil {
		return zero, err
	}
	out, err := fn(s)
	if apierrors.IsUnauthorized(err) {
		r.factory.Invalidate(cluster)
		s, err = r.bound(ctx, cluster)
		if err != nil {
			return zero, err
		}
		return fn(s)
	}
	return out, err
}

func routedErr(ctx context.Context, r *Router, cluster ports.ClusterRef, fn func(*Scheduler) error) error {
	_, err := routed(ctx, r, cluster, func(s *Scheduler) (struct{}, error) { return struct{}{}, fn(s) })
	return err
}

// DeployScenario routes to spec.Cluster.
func (r *Router) DeployScenario(ctx context.Context, spec ports.DeploySpec) error {
	return routedErr(ctx, r, spec.Cluster, func(s *Scheduler) error { return s.DeployScenario(ctx, spec) })
}

// ExecutionStatus routes to cluster.
func (r *Router) ExecutionStatus(ctx context.Context, cluster ports.ClusterRef, executionID int64, scenarios []ports.ScenarioRef) (ports.ExecutionStatus, error) {
	return routed(ctx, r, cluster, func(s *Scheduler) (ports.ExecutionStatus, error) {
		return s.ExecutionStatus(ctx, cluster, executionID, scenarios)
	})
}

// EngineDetail routes to cluster.
func (r *Router) EngineDetail(ctx context.Context, cluster ports.ClusterRef, projectID, executionID int64) (ports.ExecutionDetail, error) {
	return routed(ctx, r, cluster, func(s *Scheduler) (ports.ExecutionDetail, error) {
		return s.EngineDetail(ctx, cluster, projectID, executionID)
	})
}

// PurgeExecution routes to cluster.
func (r *Router) PurgeExecution(ctx context.Context, cluster ports.ClusterRef, executionID int64) error {
	return routedErr(ctx, r, cluster, func(s *Scheduler) error { return s.PurgeExecution(ctx, cluster, executionID) })
}

// PodLog routes to cluster.
func (r *Router) PodLog(ctx context.Context, cluster ports.ClusterRef, executionID, scenarioID int64, shard int) (string, error) {
	return routed(ctx, r, cluster, func(s *Scheduler) (string, error) {
		return s.PodLog(ctx, cluster, executionID, scenarioID, shard)
	})
}

// DeployedExecutions routes to cluster.
func (r *Router) DeployedExecutions(ctx context.Context, cluster ports.ClusterRef) (map[int64]time.Time, error) {
	return routed(ctx, r, cluster, func(s *Scheduler) (map[int64]time.Time, error) {
		return s.DeployedExecutions(ctx, cluster)
	})
}

// NodePools routes to cluster.
func (r *Router) NodePools(ctx context.Context, cluster ports.ClusterRef) ([]ports.NodePool, error) {
	return routed(ctx, r, cluster, func(s *Scheduler) ([]ports.NodePool, error) {
		return s.NodePools(ctx, cluster)
	})
}
