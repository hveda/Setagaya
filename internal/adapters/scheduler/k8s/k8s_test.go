package k8s_test

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	k8sadapter "github.com/heridotlife/Setagaya/internal/adapters/scheduler/k8s"
	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/schedulertest"
)

const ns = "honryu"

// readyEngines simulates the StatefulSet controller: it creates the ordinal
// pods a scenario would spawn and marks them Running+Ready so the adapter's
// readiness queries see them. Project id is recovered from the deployed set.
func readyEngines(t *testing.T, client *fake.Clientset, executionID, scenarioID int64, engines int) {
	t.Helper()
	ctx := context.Background()
	sel := fmt.Sprintf("execution=%d,scenario=%d", executionID, scenarioID)
	sets, err := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil || len(sets.Items) == 0 {
		t.Fatalf("no statefulset for execution %d scenario %d: %v", executionID, scenarioID, err)
	}
	set := sets.Items[0]
	name := set.Name
	for i := 0; i < engines; i++ {
		labels := map[string]string{}
		for k, v := range set.Spec.Template.Labels {
			labels[k] = v
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", name, i),
				Namespace: ns,
				Labels:    labels,
			},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}
		if _, err := client.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create pod: %v", err)
		}
	}
}

func TestK8sScheduler_Contract(t *testing.T) {
	t.Parallel()
	schedulertest.RunSchedulerContract(t, func(t *testing.T) schedulertest.Harness {
		client := fake.NewSimpleClientset()
		return schedulertest.Harness{
			Scheduler: k8sadapter.New(client, k8sadapter.Config{Namespace: ns}),
			Ready: func(executionID, scenarioID int64, engines int) {
				readyEngines(t, client, executionID, scenarioID, engines)
			},
		}
	})
}

func TestK8sScheduler_DeployIsIdempotentAndScales(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	spec := ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Engines: 2, Image: "jmeter", CPU: "500m", Memory: "256Mi"}
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("deploy 1: %v", err)
	}
	// Re-deploy with more engines: no duplicate object, replicas updated.
	spec.Engines = 5
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("deploy 2: %v", err)
	}
	name := engine.ScenarioName(1, 2, 3)
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if *set.Spec.Replicas != 5 {
		t.Fatalf("replicas = %d, want 5", *set.Spec.Replicas)
	}
	sets, _ := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if len(sets.Items) != 1 {
		t.Fatalf("statefulsets = %d, want 1 (idempotent)", len(sets.Items))
	}
	// Resource requests were applied.
	reqs := set.Spec.Template.Spec.Containers[0].Resources.Requests
	if reqs.Cpu().String() != "500m" || reqs.Memory().String() != "256Mi" {
		t.Fatalf("resources = %v", reqs)
	}
}

func TestK8sScheduler_EngineURLsFormat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns, EnginePort: 8080})

	if err := s.DeployScenario(ctx, ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Engines: 2, Image: "x"}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	urls, err := s.EngineURLs(ctx, 2, 3, 2)
	if err != nil {
		t.Fatalf("EngineURLs: %v", err)
	}
	want := "http://engine-1-2-3-0.engine-1-2-3.honryu.svc:8080"
	if urls[0] != want {
		t.Fatalf("url[0] = %q, want %q", urls[0], want)
	}
}

func TestK8sScheduler_EngineURLsUnreachableWhenAbsent(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})
	if _, err := s.EngineURLs(context.Background(), 2, 3, 2); err == nil {
		t.Fatal("EngineURLs on undeployed execution: want error, got nil")
	}
}

func TestK8sScheduler_NodePoolsGroupByLabel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns, PoolLabel: "node-pool"})

	nodes := []struct {
		name, pool string
	}{
		{"n1", "engines"}, {"n2", "engines"}, {"n3", "system"}, {"n4", ""},
	}
	for _, n := range nodes {
		labels := map[string]string{}
		if n.pool != "" {
			labels["node-pool"] = n.pool
		}
		if _, err := client.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: n.name, Labels: labels},
		}, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create node: %v", err)
		}
	}

	pools, err := s.NodePools(ctx)
	if err != nil {
		t.Fatalf("NodePools: %v", err)
	}
	sizes := map[string]int{}
	for _, p := range pools {
		sizes[p.Name] = p.Size
	}
	if sizes["engines"] != 2 || sizes["system"] != 1 || sizes["default"] != 1 {
		t.Fatalf("pool sizes = %+v", sizes)
	}
}

func TestK8sScheduler_StatusCountsOnlyReadyPods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	if err := s.DeployScenario(ctx, ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Engines: 3, Image: "x"}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	// Only 2 of 3 pods are ready; one is Pending.
	readyEngines(t, client, 2, 3, 2)
	pending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "engine-1-2-3-2", Namespace: ns, Labels: map[string]string{
			"execution": "2", "project": "1", "scenario": "3", "kind": "executor",
		}},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	if _, err := client.CoreV1().Pods(ns).Create(ctx, pending, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pending pod: %v", err)
	}

	status, err := s.ExecutionStatus(ctx, 2, []ports.ScenarioRef{{ScenarioID: 3, Engines: 3}})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Scenarios[0].EnginesDeployed != 2 || status.Scenarios[0].Reachable {
		t.Fatalf("readiness = %+v, want deployed=2 not reachable", status.Scenarios[0])
	}
}
