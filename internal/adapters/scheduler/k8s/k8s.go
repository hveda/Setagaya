// Package k8s implements ports.Scheduler on top of Kubernetes. Each scenario is
// a StatefulSet, one pod per shard, whose stable ordinals let otherwise
// identical pods pick out their own compiled config.
//
// The pods expose nothing. They ran an HTTP agent the controller called, which
// needed a headless Service and a port; now they run bzt and push their results
// out, so neither exists and no metrics scraper has a target to find.
//
// The clientset is injected, so the whole adapter is exercised in-process
// against client-go's fake clientset — no cluster, no container.
package k8s

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

const managedByLabel = "managed-by"
const managedByValue = "honryu"

// engineContainer is the pod container running bzt, as opposed to the sidecar
// beside it. A pod's logs must name it explicitly: the Kubernetes API refuses
// GetLogs on a multi-container pod that does not.
const engineContainer = "engine"

// defaultPoolLabel groups nodes into pools when Config.PoolLabel is unset.
const defaultPoolLabel = "cloud.google.com/gke-nodepool"

// defaultTerminationGrace gives bzt time to finish after SIGINT.
const defaultTerminationGrace = 30 * time.Second

// Paths inside an engine pod.
const (
	configMount    = "/honryu/config"
	kpiMount       = "/honryu/kpi"
	artifactsMount = "/honryu/artifacts"
	kpiStream      = kpiMount + "/stream.jsonl"
	// kpiExitCode is where the engine container writes bzt's exit code once it
	// finishes. It is how the sidecar learns the run is over on its own, rather
	// than only at pod teardown -- see engineScript.
	kpiExitCode = kpiMount + "/exit-code"
)

// Config tunes the adapter.
type Config struct {
	Namespace  string
	EnginePort int
	// PoolLabel is the node label whose value names the node pool.
	PoolLabel string
	// SidecarImage runs beside each engine, forwarding measurements.
	SidecarImage string
	// IngestURL is where the sidecar pushes batches.
	IngestURL string
	// TerminationGrace is how long a pod has to stop after its preStop hook
	// runs. It must exceed the time bzt needs to finish writing, or the graceful
	// stop is cut short and becomes the abrupt one it exists to avoid.
	TerminationGrace time.Duration
}

// Scheduler is the Kubernetes-backed ports.Scheduler.
type Scheduler struct {
	client       kubernetes.Interface
	ns           string
	port         int
	poolLabel    string
	sidecarImage string
	ingestURL    string
	grace        time.Duration
}

// New builds a Scheduler over the given clientset.
func New(client kubernetes.Interface, cfg Config) *Scheduler {
	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}
	port := cfg.EnginePort
	if port == 0 {
		port = 8080
	}
	poolLabel := cfg.PoolLabel
	if poolLabel == "" {
		poolLabel = defaultPoolLabel
	}
	grace := cfg.TerminationGrace
	if grace <= 0 {
		grace = defaultTerminationGrace
	}
	return &Scheduler{
		client: client, ns: ns, port: port, poolLabel: poolLabel,
		sidecarImage: cfg.SidecarImage, ingestURL: cfg.IngestURL, grace: grace,
	}
}

var _ ports.Scheduler = (*Scheduler)(nil)

// int32Bounded converts n (a port or replica count) to int32, clamping to the
// valid int32 range. The clamp is unreachable in practice — ports are <=65535
// and replica counts are small — but it makes the narrowing conversion provably
// overflow-free (gosec G115).
func int32Bounded(n int) int32 {
	switch {
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	default:
		return int32(n) // #nosec G115 -- bounds checked above
	}
}

// DeployScenario creates (or scales) the StatefulSet and headless Service for a
// scenario. It is idempotent.
func (s *Scheduler) DeployScenario(ctx context.Context, spec ports.DeploySpec) error {
	name := engine.ScenarioName(spec.ProjectID, spec.ExecutionID, spec.ScenarioID)
	labels := engine.ScenarioLabels(spec.ProjectID, spec.ExecutionID, spec.ScenarioID)
	labels[managedByLabel] = managedByValue

	// The configs must exist before the pods that mount them, or the first pods
	// start and fail before the ConfigMap lands.
	if err := s.ensureConfigMap(ctx, name, labels, spec); err != nil {
		return err
	}
	return s.ensureStatefulSet(ctx, name, labels, spec)
}

// ensureConfigMap holds every shard's compiled config, keyed by shard index.
//
// One map rather than one per pod: the pods of a StatefulSet share a template,
// so each selects its own config by its ordinal at start-up. That is what lets
// pods that are identical to Kubernetes run different fractions of the load.
func (s *Scheduler) ensureConfigMap(ctx context.Context, name string, labels map[string]string, spec ports.DeploySpec) error {
	data := make(map[string]string, len(spec.Shards)+len(spec.ScenarioFiles))
	for _, sh := range spec.Shards {
		data[shardConfigKey(sh.Index)] = string(sh.Config)
	}
	// The scenario's own artefacts ride along, so a native scenario finds the
	// script its config points at. A ConfigMap holds 1MiB, which a test plan
	// exceeds only rarely -- when it does, the deploy fails loudly here rather
	// than the pod failing quietly later.
	for name, content := range spec.ScenarioFiles {
		data[scenarioFileKey(name)] = string(content)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.ns, Labels: labels},
		Data:       data,
	}
	maps := s.client.CoreV1().ConfigMaps(s.ns)
	_, err := maps.Create(ctx, cm, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// A re-deploy replaces the configs: the shard plan may have changed.
		existing, getErr := maps.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		existing.Data = data
		_, updErr := maps.Update(ctx, existing, metav1.UpdateOptions{})
		return updErr
	}
	return err
}

// shardConfigKey names a shard's config within the ConfigMap.
func shardConfigKey(index int) string { return fmt.Sprintf("shard-%d.yml", index) }

// scenarioFileKey namespaces a scenario artefact so it cannot collide with a
// shard config.
func scenarioFileKey(name string) string { return "scenario--" + name }

func (s *Scheduler) ensureStatefulSet(ctx context.Context, name string, labels map[string]string, spec ports.DeploySpec) error {
	replicas := int32Bounded(len(spec.Shards))
	podLabels := map[string]string{}
	for k, v := range labels {
		podLabels[k] = v
	}
	podLabels["app"] = name

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.ns, Labels: labels},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: name,
			Replicas:    &replicas,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			// Shards must start together: they ramp over the same window, and a
			// staggered start would mean the aggregate never reaches the profile
			// that was asked for. Kubernetes otherwise brings StatefulSet pods up
			// one at a time, each waiting for the last to be ready.
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec:       s.podSpec(spec, name),
			},
		},
	}

	sets := s.client.AppsV1().StatefulSets(s.ns)
	_, err := sets.Create(ctx, sts, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := sets.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		existing.Spec.Replicas = &replicas
		_, updErr := sets.Update(ctx, existing, metav1.UpdateOptions{})
		return updErr
	}
	return err
}

// podSpec builds one engine pod: bzt running this shard's config, and the
// sidecar forwarding what it measures.
func (s *Scheduler) podSpec(spec ports.DeploySpec, name string) corev1.PodSpec {
	grace := int64(s.grace.Seconds())

	return corev1.PodSpec{
		// A pod is deleted with SIGTERM, which bzt does not handle: it dies at
		// once, writing no final report. The hook sends SIGINT instead, which bzt
		// does handle, and the grace period gives it time to finish.
		TerminationGracePeriodSeconds: &grace,
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: name},
						Items:                configItems(spec),
					},
				},
			},
			// The engine writes its KPI stream here and the sidecar reads it, so
			// the handover never leaves the pod.
			{Name: "kpi", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "artifacts", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		},
		Containers: []corev1.Container{
			{
				Name:    engineContainer,
				Image:   spec.Image,
				Command: []string{"/bin/sh", "-c", engineScript()},
				// No port: an engine serves nothing. It ran an HTTP agent the
				// controller called; now it runs bzt and its sidecar pushes the
				// results out, so there is nothing to connect to and nothing for
				// a metrics scraper to discover.
				VolumeMounts: []corev1.VolumeMount{
					{Name: "config", MountPath: configMount, ReadOnly: true},
					{Name: "kpi", MountPath: kpiMount},
					{Name: "artifacts", MountPath: artifactsMount},
				},
				Resources: resourceRequirements(spec),
				Lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.LifecycleHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"/bin/sh", "-c", "pkill -INT -f bzt || true"},
						},
					},
				},
			},
			{
				Name:    "sidecar",
				Image:   s.sidecarImage,
				Command: []string{"/bin/sh", "-c", s.sidecarScript(spec)},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "kpi", MountPath: kpiMount, ReadOnly: true},
				},
				// Like the engine container: /bin/sh is this container's PID 1 and
				// does not forward the pod-teardown SIGTERM to the foreground
				// sidecar process, so without this hook the sidecar never observes
				// ordinary teardown and cannot flush what it has buffered since its
				// last periodic tick. pkill reaches it directly, by command-line
				// match, bypassing PID 1 entirely.
				Lifecycle: &corev1.Lifecycle{
					PreStop: &corev1.LifecycleHandler{
						Exec: &corev1.ExecAction{
							Command: []string{"/bin/sh", "-c", "pkill -TERM -f honryu-sidecar || true"},
						},
					},
				},
				Env: []corev1.EnvVar{{
					Name: "HONRYU_INGEST_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "honryu-ingest"},
							Key:                  "token",
							Optional:             ptr(true),
						},
					},
				}},
			},
		},
	}
}

// engineScript picks this pod's config by its StatefulSet ordinal and runs bzt
// on it. Pods share a template, so the ordinal in the hostname is the only thing
// distinguishing one shard from another.
//
// bzt is run in the foreground rather than exec'd, and the container keeps
// running once it finishes. Two things depend on that:
//
//   - The StatefulSet's pods can only have restartPolicy Always -- Kubernetes
//     rejects anything else for a controller-managed pod. exec'ing bzt made it
//     the container's own process, so kubelet restarted the container the
//     instant bzt exited, cleanly or not, re-running the whole load profile.
//     Confirmed against a real cluster: a container running only `sleep 3;
//     exit 0` restarted three times within a minute, each a clean exitCode 0.
//   - Nothing captured bzt's exit code, so Honryu had no way to know how a run
//     ended (taurus.OutcomeFromExitCode existed with no feed).
//
// No set -e: bzt's own exit code must reach the `code=$?` line, and set -e
// would abort the script the moment bzt exits non-zero -- which includes an
// expected criteria failure (exit 3), not only an error.
func engineScript() string {
	argv := taurus.Command(configMount+"/shard-${ORDINAL}.yml", artifactsMount)
	return `ORDINAL="${HOSTNAME##*-}"
mkdir -p ` + kpiMount + `
` + strings.Join(argv, " ") + `
code=$?
echo "$code" > ` + kpiExitCode + `
exec tail -f /dev/null`
}

// sidecarScript runs the forwarder with this pod's identity, then keeps the
// container alive.
//
// Kubernetes' restartPolicy applies to every container in a pod alike: a
// sidecar that exits cleanly once it has pushed its final batch would be
// restarted exactly like the engine container would without the same fix, and
// would re-read the KPI stream from the start under a new stream id -- which
// the control plane would take for an unrelated stream and absorb all over
// again, doubling every measurement already pushed.
func (s *Scheduler) sidecarScript(spec ports.DeploySpec) string {
	return fmt.Sprintf(`ORDINAL="${HOSTNAME##*-}"
/honryu-sidecar -stream %s -exit-code %s -ingest-url %q -execution-id %d -scenario-id %d -shard-index "${ORDINAL}"
exec tail -f /dev/null`,
		kpiStream, kpiExitCode, s.ingestURL, spec.ExecutionID, spec.ScenarioID)
}

// configItems maps ConfigMap entries onto the paths a pod expects: shard
// configs at the root, scenario artefacts under scenario/ with their original
// names, since a compiled config points at them by name.
func configItems(spec ports.DeploySpec) []corev1.KeyToPath {
	items := make([]corev1.KeyToPath, 0, len(spec.Shards)+len(spec.ScenarioFiles))
	for _, sh := range spec.Shards {
		key := shardConfigKey(sh.Index)
		items = append(items, corev1.KeyToPath{Key: key, Path: key})
	}
	names := make([]string, 0, len(spec.ScenarioFiles))
	for name := range spec.ScenarioFiles {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic, so a redeploy does not churn the spec
	for _, name := range names {
		items = append(items, corev1.KeyToPath{Key: scenarioFileKey(name), Path: "scenario/" + name})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func ptr[T any](v T) *T { return &v }

func resourceRequirements(spec ports.DeploySpec) corev1.ResourceRequirements {
	list := corev1.ResourceList{}
	if q, err := resource.ParseQuantity(spec.CPU); err == nil && spec.CPU != "" {
		list[corev1.ResourceCPU] = q
	}
	if q, err := resource.ParseQuantity(spec.Memory); err == nil && spec.Memory != "" {
		list[corev1.ResourceMemory] = q
	}
	if len(list) == 0 {
		return corev1.ResourceRequirements{}
	}
	return corev1.ResourceRequirements{Requests: list, Limits: list}
}

// ExecutionStatus counts ready pods per scenario.
func (s *Scheduler) ExecutionStatus(ctx context.Context, _ ports.ClusterRef, executionID int64, plans []ports.ScenarioRef) (ports.ExecutionStatus, error) {
	status := ports.ExecutionStatus{}
	for _, ref := range plans {
		ready, err := s.readyPods(ctx, executionID, ref.ScenarioID)
		if err != nil {
			return ports.ExecutionStatus{}, err
		}
		pr := ports.ScenarioReadiness{
			ScenarioID:      ref.ScenarioID,
			EnginesWanted:   ref.Shards,
			EnginesDeployed: ready,
			Reachable:       ref.Shards > 0 && ready >= ref.Shards,
		}
		status.PoolSize += ready
		status.Scenarios = append(status.Scenarios, pr)
	}
	return status, nil
}

func (s *Scheduler) readyPods(ctx context.Context, executionID, scenarioID int64) (int, error) {
	sel := fmt.Sprintf("execution=%d,scenario=%d", executionID, scenarioID)
	pods, err := s.client.CoreV1().Pods(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return 0, err
	}
	ready := 0
	for i := range pods.Items {
		if podReady(&pods.Items[i]) {
			ready++
		}
	}
	return ready, nil
}

func podReady(p *corev1.Pod) bool {
	if p.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// EngineDetail lists all engine pods of a execution.
func (s *Scheduler) EngineDetail(ctx context.Context, _ ports.ClusterRef, projectID, executionID int64) (ports.ExecutionDetail, error) {
	sel := fmt.Sprintf("project=%d,execution=%d", projectID, executionID)
	pods, err := s.client.CoreV1().Pods(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return ports.ExecutionDetail{}, err
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	detail := ports.ExecutionDetail{}
	for i := range pods.Items {
		p := &pods.Items[i]
		detail.Engines = append(detail.Engines, ports.EngineDetail{
			Name:        p.Name,
			Status:      string(p.Status.Phase),
			CreatedTime: p.CreationTimestamp.Time,
		})
	}
	return detail, nil
}

// PurgeExecution deletes every StatefulSet, Service, and Pod of a execution.
func (s *Scheduler) PurgeExecution(ctx context.Context, _ ports.ClusterRef, executionID int64) error {
	sel := fmt.Sprintf("execution=%d,%s=%s", executionID, managedByLabel, managedByValue)
	opts := metav1.ListOptions{LabelSelector: sel}
	del := metav1.DeleteOptions{}

	sets, err := s.client.AppsV1().StatefulSets(s.ns).List(ctx, opts)
	if err != nil {
		return err
	}
	for i := range sets.Items {
		if err := s.client.AppsV1().StatefulSets(s.ns).Delete(ctx, sets.Items[i].Name, del); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	svcs, err := s.client.CoreV1().Services(s.ns).List(ctx, opts)
	if err != nil {
		return err
	}
	for i := range svcs.Items {
		if err := s.client.CoreV1().Services(s.ns).Delete(ctx, svcs.Items[i].Name, del); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Pods normally cascade from the StatefulSet; delete explicitly so state is
	// consistent under the fake clientset too.
	// DeleteCollection here is the Kubernetes client API (delete a execution of
	// pods), not a Honryu concept -- it keeps its upstream name.
	return s.client.CoreV1().Pods(s.ns).DeleteCollection(ctx, del, metav1.ListOptions{LabelSelector: fmt.Sprintf("execution=%d", executionID)})
}

// PodLog returns the engine container's logs from one shard's pod.
//
// The pod is found by its StatefulSet ordinal suffix rather than by name
// reconstruction, since the name embeds the project id this method is not
// given. The engine container is named explicitly: a pod with more than one
// container -- every engine pod, since the sidecar shares it -- is rejected by
// the Kubernetes API otherwise.
func (s *Scheduler) PodLog(ctx context.Context, _ ports.ClusterRef, executionID, scenarioID int64, shard int) (string, error) {
	sel := fmt.Sprintf("execution=%d,scenario=%d", executionID, scenarioID)
	pods, err := s.client.CoreV1().Pods(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return "", err
	}
	suffix := fmt.Sprintf("-%d", shard)
	for i := range pods.Items {
		if !strings.HasSuffix(pods.Items[i].Name, suffix) {
			continue
		}
		req := s.client.CoreV1().Pods(s.ns).GetLogs(pods.Items[i].Name, &corev1.PodLogOptions{Container: engineContainer})
		body, err := req.DoRaw(ctx)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	return "", ports.ErrEnginesUnreachable
}

// NodePools groups cluster nodes by their pool label, reporting each pool's
// size and earliest node creation time.
func (s *Scheduler) NodePools(ctx context.Context, _ ports.ClusterRef) ([]ports.NodePool, error) {
	nodes, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	byPool := map[string]*ports.NodePool{}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		name := n.Labels[s.poolLabel]
		if name == "" {
			name = "default"
		}
		created := n.CreationTimestamp.Time
		if pool, ok := byPool[name]; ok {
			pool.Size++
			if created.Before(pool.LaunchTime) {
				pool.LaunchTime = created
			}
		} else {
			byPool[name] = &ports.NodePool{Name: name, Size: 1, LaunchTime: created}
		}
	}
	pools := make([]ports.NodePool, 0, len(byPool))
	for _, p := range byPool {
		pools = append(pools, *p)
	}
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	return pools, nil
}

// DeployedExecutions maps execution id to its earliest StatefulSet creation.
func (s *Scheduler) DeployedExecutions(ctx context.Context, _ ports.ClusterRef) (map[int64]time.Time, error) {
	sel := fmt.Sprintf("%s=%s", managedByLabel, managedByValue)
	sets, err := s.client.AppsV1().StatefulSets(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	out := map[int64]time.Time{}
	for i := range sets.Items {
		var cid int64
		if _, err := fmt.Sscanf(sets.Items[i].Labels["execution"], "%d", &cid); err != nil {
			continue
		}
		created := sets.Items[i].CreationTimestamp.Time
		if cur, ok := out[cid]; !ok || created.Before(cur) {
			out[cid] = created
		}
	}
	return out, nil
}
