package k8s

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/ports"
	fakerepo "github.com/heridotlife/honryu/internal/ports/fake"
)

const factoryNS = "honryu"

// registerCluster materializes a credential Secret in home and stores a
// registry entry pointing at it, as clusterapp's BYOC flow would.
func registerCluster(t *testing.T, home kubernetes.Interface, registry ports.ClusterRegistry, name, apiURL, token string) {
	t.Helper()
	ctx := context.Background()
	secretName := CredentialSecretName(name)
	if err := MaterializeCredential(ctx, home, factoryNS, secretName, Credential{APIURL: apiURL, CACert: []byte("ca"), Token: token}); err != nil {
		t.Fatalf("MaterializeCredential: %v", err)
	}
	if err := registry.CreateCluster(ctx, clusterregistry.Cluster{
		Name: name, APIURL: apiURL, CACert: "ca", IngestURL: "http://ingest", SidecarImage: "img",
		Namespace: factoryNS, SecretRef: secretName, Origin: clusterregistry.OriginOperator,
	}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
}

func TestClientFactory_DefaultRefIsHomeClient(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	f := NewClientFactory(home, factoryNS, fakerepo.NewStore())

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
	f := NewClientFactory(home, factoryNS, registry)
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

func TestClientFactory_CachesAndRebuildsOnInvalidate(t *testing.T) {
	t.Parallel()
	home := fake.NewSimpleClientset()
	registry := fakerepo.NewStore()
	registerCluster(t, home, registry, "prod-eu", "https://prod-eu:6443", "tok")

	builds := 0
	f := NewClientFactory(home, factoryNS, registry)
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

func TestClientFactory_UnknownRefErrors(t *testing.T) {
	t.Parallel()
	f := NewClientFactory(fake.NewSimpleClientset(), factoryNS, fakerepo.NewStore())
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
	f := NewClientFactory(home, factoryNS, registry)
	f.build = func(*rest.Config) (kubernetes.Interface, error) { return target, nil }
	router := NewRouter(f, Config{Namespace: factoryNS})

	spec := ports.DeploySpec{
		Cluster: "prod-eu", ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "jmeter",
		Shards: []ports.ShardSpec{{Index: 0, Config: []byte("cfg"), Concurrency: 1}},
	}
	if err := router.DeployScenario(context.Background(), spec); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}

	name := engine.ScenarioName(1, 2, 3)
	if _, err := target.AppsV1().StatefulSets(factoryNS).Get(context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("StatefulSet not on the target cluster client: %v", err)
	}
	// The home client must be untouched by a non-default deploy.
	sets, err := home.AppsV1().StatefulSets(factoryNS).List(context.Background(), metav1.ListOptions{})
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
	f := NewClientFactory(home, factoryNS, fakerepo.NewStore())
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
