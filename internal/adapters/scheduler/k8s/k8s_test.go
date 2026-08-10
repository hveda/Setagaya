package k8s_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	k8sadapter "github.com/heridotlife/honryu/internal/adapters/scheduler/k8s"
	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/schedulertest"
)

const ns = "honryu"

// configHashAnnotationKey mirrors the unexported k8s.configHashAnnotation
// literal -- an external test package cannot reach the constant itself.
const configHashAnnotationKey = "honryu.dev/config-hash"

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

	spec := ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Shards: deployShards(2), Image: "jmeter", CPU: "500m", Memory: "256Mi"}
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("deploy 1: %v", err)
	}
	// Re-deploy with more shards: no duplicate object, replicas updated.
	spec.Shards = deployShards(5)
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

	pools, err := s.NodePools(ctx, "")
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

	if err := s.DeployScenario(ctx, ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Shards: deployShards(3), Image: "x"}); err != nil {
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

	status, err := s.ExecutionStatus(ctx, "", 2, []ports.ScenarioRef{{ScenarioID: 3, Shards: 3}})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Scenarios[0].EnginesDeployed != 2 || status.Scenarios[0].Reachable {
		t.Fatalf("readiness = %+v, want deployed=2 not reachable", status.Scenarios[0])
	}
}

// deployShards builds n placeholder shard specs for a deploy.
func deployShards(n int) []ports.ShardSpec {
	out := make([]ports.ShardSpec, n)
	for i := range out {
		out[i] = ports.ShardSpec{Index: i, Concurrency: 1}
	}
	return out
}

// A pod per shard is the whole point of the reshape: replicas follow the shard
// plan, so asking for four shards deploys four pods rather than a count that
// happens to agree.
func TestK8sScheduler_ReplicasFollowTheShardPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	spec := ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine",
		Shards: []ports.ShardSpec{
			{Index: 0, Concurrency: 4},
			{Index: 1, Concurrency: 3},
			{Index: 2, Concurrency: 3},
		},
	}
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if set.Spec.Replicas == nil || *set.Spec.Replicas != 3 {
		t.Errorf("replicas = %v, want 3 (one per shard)", set.Spec.Replicas)
	}

	// A deploy with no shards asks for no pods, rather than defaulting to one.
	empty := spec
	empty.ScenarioID = 4
	empty.Shards = nil
	if err := s.DeployScenario(ctx, empty); err != nil {
		t.Fatalf("deploy with no shards: %v", err)
	}
	set, err = client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 4), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if set.Spec.Replicas == nil || *set.Spec.Replicas != 0 {
		t.Errorf("replicas = %v, want 0", set.Spec.Replicas)
	}
}

// The cluster reference is accepted on every method today and selects a registry
// entry once there is more than one cluster. Passing one must not change what a
// single-cluster deployment does.
func TestK8sScheduler_ClusterRefIsAcceptedOnEveryMethod(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		Cluster: "eu-west", ProjectID: 1, ExecutionID: 2, ScenarioID: 3,
		Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if _, err := s.ExecutionStatus(ctx, "eu-west", 2, []ports.ScenarioRef{{ScenarioID: 3, Shards: 1}}); err != nil {
		t.Errorf("ExecutionStatus: %v", err)
	}
	if _, err := s.EngineDetail(ctx, "eu-west", 1, 2); err != nil {
		t.Errorf("EngineDetail: %v", err)
	}
	if _, err := s.DeployedExecutions(ctx, "eu-west"); err != nil {
		t.Errorf("DeployedExecutions: %v", err)
	}
	if _, err := s.NodePools(ctx, "eu-west"); err != nil {
		t.Errorf("NodePools: %v", err)
	}
	if err := s.PurgeExecution(ctx, "eu-west", 2); err != nil {
		t.Errorf("PurgeExecution: %v", err)
	}
}

// Shards ramp over the same window, so they must start together. Kubernetes
// otherwise brings StatefulSet pods up one at a time, each waiting for the last
// to be ready -- with a 30s ramp and eight shards the load would never reach the
// profile that was asked for.
func TestK8sScheduler_ShardsStartInParallel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(4),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if set.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
		t.Errorf("PodManagementPolicy = %q, want Parallel", set.Spec.PodManagementPolicy)
	}
}

// Each pod runs a different fraction of the load, but the pods of a StatefulSet
// share one template. The configs travel in a ConfigMap keyed by shard, and each
// pod selects its own by ordinal.
func TestK8sScheduler_EachShardGetsItsOwnConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	spec := ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine",
		Shards: []ports.ShardSpec{
			{Index: 0, Concurrency: 4, Config: []byte("execution:\n  - concurrency: 4\n")},
			{Index: 1, Concurrency: 3, Config: []byte("execution:\n  - concurrency: 3\n")},
		},
	}
	if err := s.DeployScenario(ctx, spec); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	name := engine.ScenarioName(1, 2, 3)
	cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if got := cm.Data["shard-0.yml"]; !strings.Contains(got, "concurrency: 4") {
		t.Errorf("shard 0 config = %q", got)
	}
	if got := cm.Data["shard-1.yml"]; !strings.Contains(got, "concurrency: 3") {
		t.Errorf("shard 1 config = %q", got)
	}

	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	engineC := containerNamed(t, set, "engine")
	script := strings.Join(engineC.Command, " ")
	if !strings.Contains(script, "ORDINAL") || !strings.Contains(script, "shard-${ORDINAL}.yml") {
		t.Errorf("engine command does not select a config by ordinal: %q", script)
	}
	if !strings.Contains(script, "bzt") {
		t.Errorf("engine command does not run bzt: %q", script)
	}
}

// bzt's JMeter executor saves a "modified" JMX beside the original script
// whenever the compiled config carries concurrency/throughput overrides --
// true for every native scenario -- which fails outright against a
// ConfigMap volume (Kubernetes offers no writable variant). The engine
// container must instead get a writable copy of the config to run against,
// seeded from the read-only ConfigMap mount before bzt ever starts.
func TestK8sScheduler_EngineConfigIsWritable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	name := engine.ScenarioName(1, 2, 3)
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	engineC := containerNamed(t, set, "engine")

	var configSrc, config *corev1.VolumeMount
	for i, m := range engineC.VolumeMounts {
		switch m.Name {
		case "config-src":
			configSrc = &engineC.VolumeMounts[i]
		case "config":
			config = &engineC.VolumeMounts[i]
		}
	}
	if configSrc == nil {
		t.Fatal("engine does not mount the ConfigMap's own read-only volume (config-src)")
	}
	if !configSrc.ReadOnly {
		t.Error("config-src is not read-only -- it is the ConfigMap volume, which Kubernetes never allows to be writable")
	}
	if config == nil {
		t.Fatal("engine does not mount a separate writable config volume")
	}
	if config.ReadOnly {
		t.Error("config is read-only -- bzt's in-place JMX rewrite will fail against it")
	}

	// The pod's own volume for "config" must actually be an EmptyDir (writable),
	// not the ConfigMap itself under a second name.
	var configVol *corev1.Volume
	for i, v := range set.Spec.Template.Spec.Volumes {
		if v.Name == "config" {
			configVol = &set.Spec.Template.Spec.Volumes[i]
		}
	}
	if configVol == nil || configVol.EmptyDir == nil {
		t.Fatalf("config volume = %+v, want an EmptyDir", configVol)
	}

	script := strings.Join(engineC.Command, " ")
	if !strings.Contains(script, "cp -r /honryu/config-src/. /honryu/config") {
		t.Errorf("engine command does not seed the writable config from config-src before running bzt: %q", script)
	}
}

// bzt is run rather than exec'd, and the container keeps running once it
// finishes -- otherwise its pod, a StatefulSet member and so restartPolicy
// Always by Kubernetes' own rule, would be restarted the instant bzt exited and
// silently re-run the whole load profile, and nothing would capture the exit
// code Honryu needs to know how the run ended.
func TestK8sScheduler_EngineCapturesExitCodeAndDoesNotExit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	engineC := containerNamed(t, set, "engine")
	script := strings.Join(engineC.Command, " ")

	if strings.Contains(script, "exec bzt") || strings.HasPrefix(strings.TrimSpace(script), "exec ") {
		t.Errorf("bzt is exec'd, so the container exits (and is restarted) the instant it does: %q", script)
	}
	if !strings.Contains(script, "code=$?") {
		t.Errorf("bzt's exit code is not captured: %q", script)
	}
	if !strings.Contains(script, "/honryu/kpi/exit-code") {
		t.Errorf("the captured exit code is not written where the sidecar can read it: %q", script)
	}
	if !strings.Contains(script, "exec tail -f /dev/null") {
		t.Errorf("the container does not stay alive once bzt finishes: %q", script)
	}
	// set -e would abort the script the moment bzt exits non-zero -- including an
	// expected criteria failure (exit 3) -- before code=$? is ever reached.
	if strings.Contains(script, "set -e") {
		t.Errorf("set -e would discard bzt's exit code on a non-zero exit: %q", script)
	}
}

// A container that exits cleanly after pushing its final batch would be
// restarted exactly like the engine would without the same fix, at which point
// it re-reads the KPI stream from the start under a new stream id -- which the
// control plane takes for an unrelated stream and absorbs all over again.
func TestK8sScheduler_SidecarDoesNotExitAfterItsFinalPush(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{
		Namespace: ns, SidecarImage: "honryu/sidecar:1", IngestURL: "http://control/api/ingest",
	})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	side := containerNamed(t, set, "sidecar")
	script := strings.Join(side.Command, " ")

	if !strings.Contains(script, "-exit-code /honryu/kpi/exit-code") {
		t.Errorf("sidecar is not told where to find the engine's exit code: %q", script)
	}
	if !strings.Contains(script, "exec tail -f /dev/null") {
		t.Errorf("the sidecar container does not stay alive once it finishes pushing: %q", script)
	}
}

// The container must keep running even after a crash -- restarting it would
// hit the same stale-stream double-counting problem staying alive after a
// clean exit avoids -- but a crash that leaves no trace anywhere is a shard
// that silently stops reporting with no way for an operator to learn why.
func TestK8sScheduler_SidecarLogsItsExitCodeOnCrash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{
		Namespace: ns, SidecarImage: "honryu/sidecar:1", IngestURL: "http://control/api/ingest",
	})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	side := containerNamed(t, set, "sidecar")
	script := strings.Join(side.Command, " ")

	if !strings.Contains(script, "code=$?") {
		t.Errorf("sidecar script does not capture its own exit code: %q", script)
	}
	if !strings.Contains(script, `echo "honryu-sidecar exited $code"`) {
		t.Errorf("sidecar script does not log a non-zero exit: %q", script)
	}
	// The log line must run before the container is kept alive, or a crash
	// right before that line would still vanish without a trace.
	if i, j := strings.Index(script, "honryu-sidecar exited"), strings.Index(script, "exec tail"); i < 0 || j < 0 || i > j {
		t.Errorf("crash log must precede exec tail -f /dev/null: %q", script)
	}
}

// A re-deploy may change the shard plan, so the configs are replaced rather than
// left describing the previous run.
func TestK8sScheduler_RedeployReplacesShardConfigs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})

	base := ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine"}
	first := base
	first.Shards = []ports.ShardSpec{{Index: 0, Config: []byte("first")}, {Index: 1, Config: []byte("first")}}
	if err := s.DeployScenario(ctx, first); err != nil {
		t.Fatalf("deploy 1: %v", err)
	}

	second := base
	second.Shards = []ports.ShardSpec{{Index: 0, Config: []byte("second")}}
	if err := s.DeployScenario(ctx, second); err != nil {
		t.Fatalf("deploy 2: %v", err)
	}

	cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if cm.Data["shard-0.yml"] != "second" {
		t.Errorf("shard 0 config = %q, want the new one", cm.Data["shard-0.yml"])
	}
	if _, stale := cm.Data["shard-1.yml"]; stale {
		t.Error("a shard from the previous plan still has a config")
	}
}

// A calibration search re-deploys the very same execution/scenario at a new
// QPS between steps, with the shard count typically unchanged (still one
// pinned pod). The ConfigMap's content changes, but the ConfigMap is a
// separate object the ordinary StatefulSet rolling update has no visibility
// into -- and the engine container never re-runs bzt on its own once it
// finishes (see engineScript). Without a pod-template annotation keyed to
// the config's own content, a same-shard-count re-deploy would leave an
// already-finished pod idling forever on stale config, silently producing
// no data for the new step at all.
func TestK8sScheduler_RedeployWithSameShardCountChangesPodTemplate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns})
	name := engine.ScenarioName(1, 2, 3)

	base := ports.DeploySpec{ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine"}
	first := base
	first.Shards = []ports.ShardSpec{{Index: 0, Config: []byte("qps-10")}}
	if err := s.DeployScenario(ctx, first); err != nil {
		t.Fatalf("deploy 1: %v", err)
	}
	before, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	beforeHash := before.Spec.Template.Annotations[configHashAnnotationKey]
	if beforeHash == "" {
		t.Fatal("pod template carries no config-hash annotation")
	}

	second := base
	second.Shards = []ports.ShardSpec{{Index: 0, Config: []byte("qps-40")}}
	if err := s.DeployScenario(ctx, second); err != nil {
		t.Fatalf("deploy 2: %v", err)
	}
	after, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	afterHash := after.Spec.Template.Annotations[configHashAnnotationKey]
	if afterHash == "" {
		t.Fatal("pod template carries no config-hash annotation after re-deploy")
	}
	if afterHash == beforeHash {
		t.Error("config-hash annotation unchanged despite different shard config -- a same-shard-count re-deploy would never recreate the pod")
	}
}

// Deleting a pod sends SIGTERM, which bzt does not handle -- it dies at once and
// writes nothing. The hook sends SIGINT, which it does handle, and the grace
// period has to outlast the shutdown or the hook is pointless.
func TestK8sScheduler_PodsStopGracefully(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns, TerminationGrace: 45 * time.Second})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}

	grace := set.Spec.Template.Spec.TerminationGracePeriodSeconds
	if grace == nil || *grace != 45 {
		t.Errorf("TerminationGracePeriodSeconds = %v, want 45", grace)
	}

	engineC := containerNamed(t, set, "engine")
	if engineC.Lifecycle == nil || engineC.Lifecycle.PreStop == nil || engineC.Lifecycle.PreStop.Exec == nil {
		t.Fatal("engine has no preStop hook, so a deleted pod is killed mid-write")
	}
	hook := strings.Join(engineC.Lifecycle.PreStop.Exec.Command, " ")
	if !strings.Contains(hook, "-INT") {
		t.Errorf("preStop hook = %q, want it to send SIGINT", hook)
	}
}

// /bin/sh is the sidecar container's PID 1 too, and forwards pod-teardown
// SIGTERM to the sidecar process no better than it does to bzt's -- the
// sidecar needs the same direct-signal hook the engine container has, or a
// pod deleted mid-run loses whatever the sidecar buffered since its last tick.
func TestK8sScheduler_SidecarStopsGracefully(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{
		Namespace: ns, SidecarImage: "honryu/sidecar:1", IngestURL: "http://control/api/ingest",
	})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(1),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}

	side := containerNamed(t, set, "sidecar")
	if side.Lifecycle == nil || side.Lifecycle.PreStop == nil || side.Lifecycle.PreStop.Exec == nil {
		t.Fatal("sidecar has no preStop hook, so a deleted pod loses its unflushed buffer")
	}
	hook := strings.Join(side.Lifecycle.PreStop.Exec.Command, " ")
	if !strings.Contains(hook, "honryu-sidecar") {
		t.Errorf("preStop hook = %q, want it to target the sidecar process", hook)
	}
}

// The sidecar shares the pod so the KPI handover never crosses a network, and it
// must know which shard it speaks for or measurements cannot be attributed.
func TestK8sScheduler_SidecarRunsBesideTheEngine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{
		Namespace: ns, SidecarImage: "honryu/sidecar:1", IngestURL: "http://control/api/ingest",
	})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 7, ScenarioID: 11, Image: "engine", Shards: deployShards(2),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 7, 11), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}

	side := containerNamed(t, set, "sidecar")
	if side.Image != "honryu/sidecar:1" {
		t.Errorf("sidecar image = %q", side.Image)
	}
	script := strings.Join(side.Command, " ")
	for _, want := range []string{
		"http://control/api/ingest",
		"-execution-id 7", "-scenario-id 11",
		`-shard-index "${ORDINAL}"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("sidecar command missing %q: %s", want, script)
		}
	}

	// Both containers must see the same stream, or the sidecar forwards nothing.
	if !mountsVolume(engineNamed(t, set, "engine"), "kpi") || !mountsVolume(side, "kpi") {
		t.Error("engine and sidecar do not share the kpi volume")
	}
}

func containerNamed(t *testing.T, set *appsv1.StatefulSet, name string) corev1.Container {
	t.Helper()
	return engineNamed(t, set, name)
}

func engineNamed(t *testing.T, set *appsv1.StatefulSet, name string) corev1.Container {
	t.Helper()
	for _, c := range set.Spec.Template.Spec.Containers {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no container named %q in the pod template", name)
	return corev1.Container{}
}

func mountsVolume(c corev1.Container, volume string) bool {
	for _, m := range c.VolumeMounts {
		if m.Name == volume {
			return true
		}
	}
	return false
}

// Engine pods serve nothing, so nothing can discover them as a scrape target.
// Under the agent protocol each pod exposed an HTTP port behind a headless
// Service; both are gone with it.
func TestK8sScheduler_PodsExposeNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	s := k8sadapter.New(client, k8sadapter.Config{Namespace: ns, EnginePort: 8080})

	if err := s.DeployScenario(ctx, ports.DeploySpec{
		ProjectID: 1, ExecutionID: 2, ScenarioID: 3, Image: "engine", Shards: deployShards(2),
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	services, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	if len(services.Items) != 0 {
		t.Errorf("created %d services; engine pods are not reachable and need none", len(services.Items))
	}

	set, err := client.AppsV1().StatefulSets(ns).Get(ctx, engine.ScenarioName(1, 2, 3), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	for _, c := range set.Spec.Template.Spec.Containers {
		if len(c.Ports) != 0 {
			t.Errorf("container %q exposes %v; nothing connects to an engine", c.Name, c.Ports)
		}
	}
}
