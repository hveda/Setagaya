package k8s

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/ports"
	fakerepo "github.com/heridotlife/honryu/internal/ports/fake"
)

const (
	// factoryNS is the home/control-plane namespace, where the registry's
	// credential Secrets live and where the default cluster deploys.
	factoryNS = "honryu"
	// targetNS is a registered cluster's own deploy namespace, distinct from
	// the home namespace, so tests can tell per-cluster namespace routing apart.
	targetNS = "engines"
)

// registerCluster materializes a credential Secret in the home namespace and
// stores a registry entry (deploying into targetNS) pointing at it, as
// clusterapp's BYOC flow would.
func registerCluster(t *testing.T, home kubernetes.Interface, registry ports.ClusterRegistry, name, apiURL, token string) {
	t.Helper()
	ctx := context.Background()
	secretName := CredentialSecretName(name)
	if err := MaterializeCredential(ctx, home, factoryNS, secretName, Credential{APIURL: apiURL, CACert: []byte("ca"), Token: token}); err != nil {
		t.Fatalf("MaterializeCredential: %v", err)
	}
	if err := registry.CreateCluster(ctx, clusterregistry.Cluster{
		Name: name, APIURL: apiURL, CACert: "ca", IngestURL: "http://ingest", SidecarImage: "img",
		Namespace: targetNS, SecretRef: secretName, Origin: clusterregistry.OriginOperator,
	}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
}

func TestClientFactory_DefaultRefIsHomeClient(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, fakerepo.NewStore())

	got, err := f.Client(context.Background(), "")
	if err != nil {
		t.Fatalf("Client(default): %v", err)
	}
	if got != kubernetes.Interface(home) {
		t.Fatalf("Client(default) did not return the home client")
	}
}

func TestClientFactory_NonDefaultBuildsFromSecret(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok-eu")

	target := fake.NewSimpleClientset()
	var gotHost, gotToken string
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, registry)
	f.build = func(cfg *rest.Config) (kubernetes.Interface, error) {
		gotHost, gotToken = cfg.Host, cfg.BearerToken
		return target, nil
	}

	got, err := f.Client(context.Background(), "prod-eu")
	if err != nil {
		t.Fatalf("Client(prod-eu): %v", err)
	}
	if got != kubernetes.Interface(target) {
		t.Fatalf("Client(prod-eu) did not return the built target client")
	}
	// The rest.Config was built from the registered entry's Secret.
	if gotHost != "https://prod-eu:6443" || gotToken != "tok-eu" {
		t.Fatalf("built config host/token = %q/%q, want from the Secret", gotHost, gotToken)
	}
}

// A registered cluster's namespace, sidecar image, and ingest URL come from
// its entry; the default cluster's come from DefaultDeploy.
func TestClientFactory_ResolveCarriesPerClusterDeploySettings(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok")

	// The home namespace (def.Namespace) must be factoryNS so the credential
	// Secret is found; the default cluster's sidecar/ingest are distinct.
	f := NewClientFactory(home, DefaultDeploy{
		Namespace: factoryNS, SidecarImage: "default-sidecar:1", IngestURL: "http://default-ingest",
	}, registry)
	f.build = func(*rest.Config) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil }
	ctx := context.Background()

	got, err := f.Resolve(ctx, "prod-eu")
	if err != nil {
		t.Fatalf("Resolve(prod-eu): %v", err)
	}
	// registerCluster stores namespace targetNS, SidecarImage "img", IngestURL "http://ingest".
	if got.namespace != targetNS || got.sidecarImage != "img" || got.ingestURL != "http://ingest" {
		t.Fatalf("resolved settings = %+v, want the entry's (ns=%s img=img ingest=http://ingest)", got, targetNS)
	}

	def, err := f.Resolve(ctx, "")
	if err != nil {
		t.Fatalf("Resolve(default): %v", err)
	}
	if def.namespace != factoryNS || def.sidecarImage != "default-sidecar:1" || def.ingestURL != "http://default-ingest" {
		t.Fatalf("default resolved settings = %+v, want DefaultDeploy", def)
	}
}

func TestClientFactory_CachesAndRebuildsOnInvalidate(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok")

	builds := 0
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, registry)
	f.build = func(*rest.Config) (kubernetes.Interface, error) {
		builds++
		return fake.NewSimpleClientset(), nil
	}
	ctx := context.Background()

	if _, err := f.Client(ctx, "prod-eu"); err != nil {
		t.Fatalf("Client 1: %v", err)
	}
	if _, err := f.Client(ctx, "prod-eu"); err != nil {
		t.Fatalf("Client 2: %v", err)
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1 (cached)", builds)
	}
	f.Invalidate("prod-eu")
	if _, err := f.Client(ctx, "prod-eu"); err != nil {
		t.Fatalf("Client 3: %v", err)
	}
	if builds != 2 {
		t.Fatalf("builds = %d after invalidate, want 2 (rebuilt)", builds)
	}
}

// Concurrent resolves of the same ref must all observe the same cached client
// (the race winner's), never a torn or displaced one, and be data-race clean
// (run under -race). The build runs without the lock held.
func TestClientFactory_ConcurrentResolveIsConsistent(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok")

	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, registry)
	f.build = func(*rest.Config) (kubernetes.Interface, error) { return fake.NewSimpleClientset(), nil }

	const n = 16
	clients := make([]kubernetes.Interface, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r, err := f.Resolve(context.Background(), "prod-eu")
			if err != nil {
				t.Errorf("Resolve: %v", err)
				return
			}
			clients[idx] = r.client
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if clients[i] != clients[0] {
			t.Fatalf("racer %d got a different client than racer 0 -- cache is inconsistent", i)
		}
	}
}

func TestClientFactory_UnknownRefErrors(t *testing.T) {
	t.Parallel()
	f := NewClientFactory(fake.NewSimpleClientset(), DefaultDeploy{Namespace: factoryNS}, fakerepo.NewStore())
	if _, err := f.Client(context.Background(), "ghost"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Client(ghost) = %v, want ErrNotFound", err)
	}
}

// A Router deploy for a non-default cluster lands on that cluster's client, not
// the home client -- the whole registry-backed path end to end.
func TestRouter_DeployScenarioRoutesToClusterClient(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok")

	target := fake.NewSimpleClientset()
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, registry)
	f.build = func(*rest.Config) (kubernetes.Interface, error) { return target, nil }
	router := NewRouter(f, Config{Namespace: factoryNS})

	spec := ports.DeploySpec{
		Cluster: "prod-eu", ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "jmeter",
		Shards: []ports.ShardSpec{{Index: 0, Config: []byte("cfg"), Concurrency: 1}},
	}
	if err := router.DeployScenario(context.Background(), spec); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}

	// The deploy lands in the entry's namespace (targetNS) on the target client.
	name := engine.ScenarioName(1, 2, 3)
	if _, err := target.AppsV1().StatefulSets(targetNS).Get(context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("StatefulSet not on the target cluster client in %s: %v", targetNS, err)
	}
	// The home client must be untouched by a non-default deploy.
	sets, err := home.AppsV1().StatefulSets("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list home statefulsets: %v", err)
	}
	if len(sets.Items) != 0 {
		t.Fatalf("home client got %d statefulsets, want 0", len(sets.Items))
	}
}

// The default ref deploys on the home client, exactly as before the factory.
func TestRouter_DeployScenarioDefaultUsesHomeClient(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, fakerepo.NewStore())
	router := NewRouter(f, Config{Namespace: factoryNS})

	spec := ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "jmeter",
		Shards: []ports.ShardSpec{{Index: 0, Config: []byte("cfg"), Concurrency: 1}},
	}
	if err := router.DeployScenario(context.Background(), spec); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}
	name := engine.ScenarioName(1, 2, 3)
	if _, err := home.AppsV1().StatefulSets(factoryNS).Get(context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("StatefulSet not on the home client for the default ref: %v", err)
	}
}

// routedCluster builds a Router with one registered cluster ("prod-eu") backed
// by its own fake clientset, ready for read/purge routing assertions. The
// returned spec deploys one scenario there when applied.
func routedCluster(t *testing.T) (router *Router, home, target kubernetes.Interface, spec ports.DeploySpec) {
	t.Helper()
	home = fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok")
	target = fake.NewSimpleClientset()
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, registry)
	f.build = func(*rest.Config) (kubernetes.Interface, error) { return target, nil }
	router = NewRouter(f, Config{Namespace: factoryNS, PoolLabel: "pool"})
	spec = ports.DeploySpec{
		Cluster: "prod-eu", ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "jmeter",
		Shards: []ports.ShardSpec{{Index: 0, Config: []byte("cfg"), Concurrency: 1}},
	}
	return router, home, target, spec
}

// A deploy on a registered cluster is visible to DeployedExecutions routed to
// that cluster and invisible to the default cluster's -- the map is read from
// the ref's own client.
func TestRouter_DeployedExecutionsRoutesToCluster(t *testing.T) {
	t.Parallel()
	router, _, _, spec := routedCluster(t)
	ctx := context.Background()
	if err := router.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}

	got, err := router.DeployedExecutions(ctx, "prod-eu")
	if err != nil {
		t.Fatalf("DeployedExecutions(prod-eu): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("DeployedExecutions(prod-eu) = %v, want exactly execution 2", got)
	}
	if _, ok := got[2]; !ok {
		t.Fatalf("DeployedExecutions(prod-eu) = %v, want key 2", got)
	}

	def, err := router.DeployedExecutions(ctx, "")
	if err != nil {
		t.Fatalf("DeployedExecutions(default): %v", err)
	}
	if len(def) != 0 {
		t.Fatalf("DeployedExecutions(default) = %v, want empty (deploy was on prod-eu)", def)
	}
}

// Each cluster reports its own node pools: nodes seeded on the target and home
// clients group separately per ref.
func TestRouter_NodePoolsRoutesToCluster(t *testing.T) {
	t.Parallel()
	router, home, target, _ := routedCluster(t)
	created := metav1.NewTime(time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC))
	for _, tc := range []struct {
		client kubernetes.Interface
		name   string
		pool   string
	}{
		{target, "spot-a", "spot"},
		{target, "spot-b", "spot"},
		{home, "onprem-a", "onprem"},
	} {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: tc.name, Labels: map[string]string{"pool": tc.pool}, CreationTimestamp: created,
		}}
		if _, err := tc.client.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create node %s: %v", tc.name, err)
		}
	}

	got, err := router.NodePools(context.Background(), "prod-eu")
	if err != nil {
		t.Fatalf("NodePools(prod-eu): %v", err)
	}
	if len(got) != 1 || got[0].Name != "spot" || got[0].Size != 2 {
		t.Fatalf("NodePools(prod-eu) = %+v, want one spot pool of size 2", got)
	}
	if !got[0].LaunchTime.Equal(created.Time) {
		t.Fatalf("NodePools(prod-eu) launch = %v, want %v", got[0].LaunchTime, created.Time)
	}

	def, err := router.NodePools(context.Background(), "")
	if err != nil {
		t.Fatalf("NodePools(default): %v", err)
	}
	if len(def) != 1 || def[0].Name != "onprem" || def[0].Size != 1 {
		t.Fatalf("NodePools(default) = %+v, want one onprem pool of size 1", def)
	}
}

// EngineDetail lists engine pods from the ref's cluster: a pod seeded on the
// target client is reported for prod-eu and absent for the default ref.
func TestRouter_EngineDetailRoutesToCluster(t *testing.T) {
	t.Parallel()
	router, _, target, _ := routedCluster(t)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "engine-2-3-0", Namespace: targetNS,
		Labels: map[string]string{"project": "1", "execution": "2"},
	}}
	if _, err := target.CoreV1().Pods(targetNS).Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed pod: %v", err)
	}

	got, err := router.EngineDetail(context.Background(), "prod-eu", 1, 2)
	if err != nil {
		t.Fatalf("EngineDetail(prod-eu): %v", err)
	}
	if len(got.Engines) != 1 || got.Engines[0].Name != "engine-2-3-0" {
		t.Fatalf("EngineDetail(prod-eu) = %+v, want the seeded pod", got.Engines)
	}

	def, err := router.EngineDetail(context.Background(), "", 1, 2)
	if err != nil {
		t.Fatalf("EngineDetail(default): %v", err)
	}
	if len(def.Engines) != 0 {
		t.Fatalf("EngineDetail(default) = %+v, want no engines", def.Engines)
	}
}

// PurgeExecution routed at a registered cluster removes that cluster's
// statefulset and is idempotent; the home client is untouched throughout.
func TestRouter_PurgeExecutionRoutesToCluster(t *testing.T) {
	t.Parallel()
	router, home, target, spec := routedCluster(t)
	ctx := context.Background()
	if err := router.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}

	if err := router.PurgeExecution(ctx, "prod-eu", spec.ExecutionID); err != nil {
		t.Fatalf("PurgeExecution(prod-eu): %v", err)
	}
	sets, err := target.AppsV1().StatefulSets(targetNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list target statefulsets: %v", err)
	}
	if len(sets.Items) != 0 {
		t.Fatalf("target has %d statefulsets after purge, want 0", len(sets.Items))
	}
	if err := router.PurgeExecution(ctx, "prod-eu", spec.ExecutionID); err != nil {
		t.Fatalf("PurgeExecution(prod-eu) again: %v (want idempotent nil)", err)
	}

	homeSets, err := home.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list home statefulsets: %v", err)
	}
	if len(homeSets.Items) != 0 {
		t.Fatalf("home client got %d statefulsets, want 0", len(homeSets.Items))
	}
}

// ExecutionStatus routed through the router reports the plan's wanted shards
// per scenario; the fake clientset has no running pods, so readiness is zero.
func TestRouter_ExecutionStatusReportsWantedShards(t *testing.T) {
	t.Parallel()
	router, _, _, spec := routedCluster(t)
	ctx := context.Background()
	if err := router.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}

	got, err := router.ExecutionStatus(ctx, "prod-eu", spec.ExecutionID, []ports.ScenarioRef{{ScenarioID: 3, Shards: 2}})
	if err != nil {
		t.Fatalf("ExecutionStatus(prod-eu): %v", err)
	}
	if len(got.Scenarios) != 1 {
		t.Fatalf("Scenarios = %+v, want one", got.Scenarios)
	}
	s := got.Scenarios[0]
	if s.ScenarioID != 3 || s.EnginesWanted != 2 || s.EnginesDeployed != 0 || s.Reachable {
		t.Fatalf("scenario readiness = %+v, want id 3 wanted 2 deployed 0 reachable false", s)
	}
	if got.PoolSize != 0 {
		t.Fatalf("PoolSize = %d, want 0 (no pods in the fake clientset)", got.PoolSize)
	}
}

// PodLog routed to a cluster with no matching shard reports the engines as
// unreachable rather than an infrastructure error.
func TestRouter_PodLogUnmatchedShardIsUnreachable(t *testing.T) {
	t.Parallel()
	router, _, _, _ := routedCluster(t)
	if _, err := router.PodLog(context.Background(), "prod-eu", 99, 98, 0); !errors.Is(err, ports.ErrEnginesUnreachable) {
		t.Fatalf("PodLog(prod-eu, no pod) = %v, want ErrEnginesUnreachable", err)
	}
}

// Every routed method fails on an unregistered cluster ref -- the registry
// lookup error propagates before any client call.
func TestRouter_UnregisteredClusterFailsEveryRoutedMethod(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	f := NewClientFactory(home, DefaultDeploy{Namespace: factoryNS}, fakerepo.NewStore())
	router := NewRouter(f, Config{Namespace: factoryNS})
	ctx := context.Background()

	calls := []struct {
		name string
		call func() error
	}{
		{"ExecutionStatus", func() error { _, err := router.ExecutionStatus(ctx, "ghost", 1, nil); return err }},
		{"EngineDetail", func() error { _, err := router.EngineDetail(ctx, "ghost", 1, 1); return err }},
		{"PurgeExecution", func() error { return router.PurgeExecution(ctx, "ghost", 1) }},
		{"PodLog", func() error { _, err := router.PodLog(ctx, "ghost", 1, 1, 0); return err }},
		{"DeployedExecutions", func() error { _, err := router.DeployedExecutions(ctx, "ghost"); return err }},
		{"NodePools", func() error { _, err := router.NodePools(ctx, "ghost"); return err }},
	}
	for _, c := range calls {
		if err := c.call(); !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("%s(ghost) = %v, want ErrNotFound", c.name, err)
		}
	}
}

// scenarioFileKey namespaces a scenario artefact so it can never collide with
// a shard-config key in the same ConfigMap.
func TestScenarioFileKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want string
	}{
		{"orders.yml", "scenario--orders.yml"},
		{"", "scenario--"},
		{"scenario--orders.yml", "scenario--scenario--orders.yml"},
	} {
		if got := scenarioFileKey(tc.name); got != tc.want {
			t.Errorf("scenarioFileKey(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
	if scenarioFileKey("shard-0.yml") == shardConfigKey(0) {
		t.Errorf("scenarioFileKey(%q) collides with shardConfigKey(0) = %q", "shard-0.yml", shardConfigKey(0))
	}
}
