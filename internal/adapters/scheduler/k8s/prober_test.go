package k8s

import (
	"context"
	"errors"
	"strings"
	"testing"

	authzv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/heridotlife/honryu/internal/ports"
)

// newProbeClient returns a fake clientset with a reachable server version and
// the given SelfSubjectAccessReview reactor installed.
func newProbeClient(t *testing.T, ssar clienttesting.ReactionFunc) *fake.Clientset {
	t.Helper()
	client := fake.NewSimpleClientset()
	client.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.0"}
	client.PrependReactor("create", "selfsubjectaccessreviews", ssar)
	return client
}

func allowAll(action clienttesting.Action) (bool, runtime.Object, error) {
	ssar := action.(clienttesting.CreateAction).GetObject().(*authzv1.SelfSubjectAccessReview)
	ssar.Status.Allowed = true
	return true, ssar, nil
}

func denyResource(resource string) clienttesting.ReactionFunc {
	return func(action clienttesting.Action) (bool, runtime.Object, error) {
		ssar := action.(clienttesting.CreateAction).GetObject().(*authzv1.SelfSubjectAccessReview)
		ssar.Status.Allowed = ssar.Spec.ResourceAttributes.Resource != resource
		return true, ssar, nil
	}
}

func TestProbeClient_AllAllowed(t *testing.T) {
	t.Parallel()
	client := newProbeClient(t, allowAll)
	if err := probeClient(context.Background(), client, "honryu"); err != nil {
		t.Fatalf("probeClient (all allowed) = %v, want nil", err)
	}
}

func TestProbeClient_UnderPrivileged(t *testing.T) {
	t.Parallel()
	client := newProbeClient(t, denyResource("configmaps"))
	err := probeClient(context.Background(), client, "honryu")
	var pe *ports.ProbeError
	if !errors.As(err, &pe) {
		t.Fatalf("probeClient err = %v, want *ports.ProbeError", err)
	}
	if pe.Kind != ports.ProbeUnderPrivileged {
		t.Fatalf("Kind = %q, want under_privileged", pe.Kind)
	}
	if !strings.Contains(pe.Message, "configmaps") || !strings.Contains(pe.Message, "honryu") {
		t.Fatalf("Message = %q, want it to name configmaps and the namespace", pe.Message)
	}
}

// An SSAR that the server rejects (401) classifies as unauthorized, not
// under-privileged.
func TestProbeClient_SSARErrorClassified(t *testing.T) {
	t.Parallel()
	client := newProbeClient(t, func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("token rejected")
	})
	err := probeClient(context.Background(), client, "honryu")
	var pe *ports.ProbeError
	if !errors.As(err, &pe) || pe.Kind != ports.ProbeUnauthorized {
		t.Fatalf("probeClient err = %v, want ProbeUnauthorized", err)
	}
}

func TestClassifyServerError(t *testing.T) {
	t.Parallel()
	gr := schema.GroupResource{Resource: "pods"}
	cases := []struct {
		name string
		err  error
		want ports.ProbeFailureKind
	}{
		{"unauthorized", apierrors.NewUnauthorized("nope"), ports.ProbeUnauthorized},
		{"forbidden", apierrors.NewForbidden(gr, "x", errors.New("denied")), ports.ProbeUnderPrivileged},
		{"generic", errors.New("dial tcp: connection refused"), ports.ProbeUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyServerError(tc.err)
			if got.Kind != tc.want {
				t.Fatalf("classifyServerError(%s).Kind = %q, want %q", tc.name, got.Kind, tc.want)
			}
			if got.Err == nil {
				t.Fatalf("classifyServerError(%s) dropped the underlying error", tc.name)
			}
		})
	}
}

// Prober satisfies ports.ClusterProber and fails closed on a nonsense
// credential (no reachable server) with a *ProbeError.
func TestProber_Probe_InvalidCredential(t *testing.T) {
	t.Parallel()
	var p Prober
	err := p.Probe(context.Background(), ports.ClusterCredential{APIURL: "https://a", CACert: []byte("ca")}, "honryu")
	var pe *ports.ProbeError
	if !errors.As(err, &pe) {
		t.Fatalf("Probe(no credential) err = %v, want *ports.ProbeError", err)
	}
	if pe.Kind != ports.ProbeUnreachable {
		t.Fatalf("Kind = %q, want unreachable", pe.Kind)
	}
}
