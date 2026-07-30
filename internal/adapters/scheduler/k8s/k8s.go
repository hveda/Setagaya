// Package k8s implements ports.Scheduler on top of Kubernetes. Each plan is a
// StatefulSet fronted by a headless Service, giving every engine a stable
// ordinal identity and DNS name (engine-<p>-<c>-<pl>-<i>). The clientset is
// injected, so the whole adapter is exercised in-process against client-go's
// fake clientset — no cluster, no container.
package k8s

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports"
)

const managedByLabel = "managed-by"
const managedByValue = "setagaya-v3"

// defaultPoolLabel groups nodes into pools when Config.PoolLabel is unset.
const defaultPoolLabel = "cloud.google.com/gke-nodepool"

// Config tunes the adapter.
type Config struct {
	Namespace  string
	EnginePort int
	// PoolLabel is the node label whose value names the node pool.
	PoolLabel string
}

// Scheduler is the Kubernetes-backed ports.Scheduler.
type Scheduler struct {
	client    kubernetes.Interface
	ns        string
	port      int
	poolLabel string
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
	return &Scheduler{client: client, ns: ns, port: port, poolLabel: poolLabel}
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
// plan. It is idempotent.
func (s *Scheduler) DeployScenario(ctx context.Context, spec ports.DeploySpec) error {
	name := engine.PlanName(spec.ProjectID, spec.ExecutionID, spec.ScenarioID)
	labels := engine.PlanLabels(spec.ProjectID, spec.ExecutionID, spec.ScenarioID)
	labels[managedByLabel] = managedByValue

	if err := s.ensureService(ctx, name, labels); err != nil {
		return err
	}
	return s.ensureStatefulSet(ctx, name, labels, spec)
}

func (s *Scheduler) ensureService(ctx context.Context, name string, labels map[string]string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.ns, Labels: labels},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone, // headless
			Selector:  map[string]string{"app": name},
			Ports:     []corev1.ServicePort{{Port: int32Bounded(s.port), Name: "agent"}},
		},
	}
	_, err := s.client.CoreV1().Services(s.ns).Create(ctx, svc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (s *Scheduler) ensureStatefulSet(ctx context.Context, name string, labels map[string]string, spec ports.DeploySpec) error {
	replicas := int32Bounded(spec.Engines)
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
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:      "engine",
						Image:     spec.Image,
						Ports:     []corev1.ContainerPort{{ContainerPort: int32Bounded(s.port)}},
						Resources: resourceRequirements(spec),
					}},
				},
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

// EngineURLs returns the stable per-engine DNS URLs, or ErrEnginesUnreachable
// when the plan's Service is absent.
func (s *Scheduler) EngineURLs(ctx context.Context, executionID, scenarioID int64, engines int) ([]string, error) {
	projectID, err := s.projectOf(ctx, executionID)
	if err != nil {
		return nil, err
	}
	name := engine.PlanName(projectID, executionID, scenarioID)
	if _, err := s.client.CoreV1().Services(s.ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		return nil, ports.ErrEnginesUnreachable
	}
	urls := make([]string, engines)
	for i := range urls {
		urls[i] = fmt.Sprintf("http://%s-%d.%s.%s.svc:%d", name, i, name, s.ns, s.port)
	}
	return urls, nil
}

// projectOf recovers the project id from any pod/statefulset labelled with the
// collection; deploy specs carry the project but query methods do not.
func (s *Scheduler) projectOf(ctx context.Context, executionID int64) (int64, error) {
	sel := fmt.Sprintf("collection=%d,%s=%s", executionID, managedByLabel, managedByValue)
	sets, err := s.client.AppsV1().StatefulSets(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return 0, err
	}
	if len(sets.Items) == 0 {
		return 0, ports.ErrEnginesUnreachable
	}
	var projectID int64
	_, _ = fmt.Sscanf(sets.Items[0].Labels["project"], "%d", &projectID)
	return projectID, nil
}

// ExecutionStatus counts ready pods per plan.
func (s *Scheduler) ExecutionStatus(ctx context.Context, executionID int64, plans []ports.ScenarioRef) (ports.ExecutionStatus, error) {
	status := ports.ExecutionStatus{}
	for _, ref := range plans {
		ready, err := s.readyPods(ctx, executionID, ref.ScenarioID)
		if err != nil {
			return ports.ExecutionStatus{}, err
		}
		pr := ports.ScenarioReadiness{
			ScenarioID:      ref.ScenarioID,
			EnginesWanted:   ref.Engines,
			EnginesDeployed: ready,
			Reachable:       ref.Engines > 0 && ready >= ref.Engines,
		}
		status.PoolSize += ready
		status.Scenarios = append(status.Scenarios, pr)
	}
	return status, nil
}

func (s *Scheduler) readyPods(ctx context.Context, executionID, scenarioID int64) (int, error) {
	sel := fmt.Sprintf("collection=%d,plan=%d", executionID, scenarioID)
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

// EngineDetail lists all engine pods of a collection.
func (s *Scheduler) EngineDetail(ctx context.Context, projectID, executionID int64) (ports.ExecutionDetail, error) {
	sel := fmt.Sprintf("project=%d,collection=%d", projectID, executionID)
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

// PurgeExecution deletes every StatefulSet, Service, and Pod of a collection.
func (s *Scheduler) PurgeExecution(ctx context.Context, executionID int64) error {
	sel := fmt.Sprintf("collection=%d,%s=%s", executionID, managedByLabel, managedByValue)
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
	// DeleteCollection here is the Kubernetes client API (delete a collection of
	// pods), not a Honryu concept -- it keeps its upstream name.
	return s.client.CoreV1().Pods(s.ns).DeleteCollection(ctx, del, metav1.ListOptions{LabelSelector: fmt.Sprintf("collection=%d", executionID)})
}

// PodLog returns the logs of a plan's first engine pod.
func (s *Scheduler) PodLog(ctx context.Context, executionID, scenarioID int64) (string, error) {
	sel := fmt.Sprintf("collection=%d,plan=%d", executionID, scenarioID)
	pods, err := s.client.CoreV1().Pods(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", ports.ErrEnginesUnreachable
	}
	req := s.client.CoreV1().Pods(s.ns).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{})
	body, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// NodePools groups cluster nodes by their pool label, reporting each pool's
// size and earliest node creation time.
func (s *Scheduler) NodePools(ctx context.Context) ([]ports.NodePool, error) {
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

// DeployedExecutions maps collection id to its earliest StatefulSet creation.
func (s *Scheduler) DeployedExecutions(ctx context.Context) (map[int64]time.Time, error) {
	sel := fmt.Sprintf("%s=%s", managedByLabel, managedByValue)
	sets, err := s.client.AppsV1().StatefulSets(s.ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	out := map[int64]time.Time{}
	for i := range sets.Items {
		var cid int64
		if _, err := fmt.Sscanf(sets.Items[i].Labels["collection"], "%d", &cid); err != nil {
			continue
		}
		created := sets.Items[i].CreationTimestamp.Time
		if cur, ok := out[cid]; !ok || created.Before(cur) {
			out[cid] = created
		}
	}
	return out, nil
}
