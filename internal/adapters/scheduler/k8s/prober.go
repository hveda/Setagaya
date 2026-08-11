package k8s

import (
	"context"
	"fmt"

	authzv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/heridotlife/honryu/internal/ports"
)

// Prober implements ports.ClusterProber against a real Kubernetes API server.
type Prober struct{}

var _ ports.ClusterProber = Prober{}

// Probe builds a client from cred and checks the target cluster: reachable and
// authenticated (a discovery call), then authorized for the least-privilege
// verbs the scheduler needs in namespace. It returns a *ports.ProbeError.
func (Prober) Probe(ctx context.Context, cred ports.ClusterCredential, namespace string) error {
	cfg, err := RestConfigFromCredential(Credential{
		APIURL:     cred.APIURL,
		CACert:     cred.CACert,
		Token:      cred.Token,
		ClientCert: cred.ClientCert,
		ClientKey:  cred.ClientKey,
	})
	if err != nil {
		return &ports.ProbeError{Kind: ports.ProbeUnreachable, Message: "invalid credential", Err: err}
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return &ports.ProbeError{Kind: ports.ProbeUnreachable, Message: "build client", Err: err}
	}
	return probeClient(ctx, client, namespace)
}

// accessCheck is one (verb, resource) authorization question for the namespace.
type accessCheck struct {
	verb        string
	group       string
	resource    string
	subresource string
}

func (c accessCheck) describe() string {
	if c.subresource != "" {
		return c.resource + "/" + c.subresource
	}
	return c.resource
}

// leastPrivilegeChecks are the verbs the k8s scheduler actually exercises:
// it creates + lists StatefulSets and ConfigMaps, lists pods, and reads pod
// logs. Pod creation is the StatefulSet controller's job, not Honryu's, so it
// is deliberately not required -- checking it would reject a correctly-scoped
// least-privilege Role.
var leastPrivilegeChecks = []accessCheck{
	{verb: "create", group: "apps", resource: "statefulsets"},
	{verb: "list", group: "apps", resource: "statefulsets"},
	{verb: "create", group: "", resource: "configmaps"},
	{verb: "list", group: "", resource: "configmaps"},
	{verb: "list", group: "", resource: "pods"},
	{verb: "get", group: "", resource: "pods", subresource: "log"},
}

// probeClient runs the reachability + authorization checks against any
// kubernetes.Interface, so it is exercised against the fake clientset.
func probeClient(ctx context.Context, client kubernetes.Interface, namespace string) error {
	if _, err := client.Discovery().ServerVersion(); err != nil {
		return classifyServerError(err)
	}
	for _, c := range leastPrivilegeChecks {
		allowed, err := can(ctx, client, namespace, c)
		if err != nil {
			return classifyServerError(err)
		}
		if !allowed {
			return &ports.ProbeError{
				Kind:    ports.ProbeUnderPrivileged,
				Message: fmt.Sprintf("cannot %s %s in namespace %q", c.verb, c.describe(), namespace),
			}
		}
	}
	return nil
}

// can asks the API server whether the caller may perform c in namespace, via a
// SelfSubjectAccessReview -- a permission check with no side effect, unlike
// actually creating a resource.
func can(ctx context.Context, client kubernetes.Interface, namespace string, c accessCheck) (bool, error) {
	ssar := &authzv1.SelfSubjectAccessReview{
		Spec: authzv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authzv1.ResourceAttributes{
				Namespace:   namespace,
				Verb:        c.verb,
				Group:       c.group,
				Resource:    c.resource,
				Subresource: c.subresource,
			},
		},
	}
	res, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, ssar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return res.Status.Allowed, nil
}

// classifyServerError maps a k8s API error onto a ProbeError kind: a rejected
// credential is unauthorized, a forbidden response is under-privileged, and
// anything else (network, DNS, TLS, 5xx) is unreachable.
func classifyServerError(err error) *ports.ProbeError {
	switch {
	case apierrors.IsUnauthorized(err):
		return &ports.ProbeError{Kind: ports.ProbeUnauthorized, Message: "authentication rejected", Err: err}
	case apierrors.IsForbidden(err):
		return &ports.ProbeError{Kind: ports.ProbeUnderPrivileged, Message: "access forbidden", Err: err}
	default:
		return &ports.ProbeError{Kind: ports.ProbeUnreachable, Message: "cannot reach cluster", Err: err}
	}
}
