package ports

import (
	"context"
	"fmt"
)

// ClusterCredential is the provider-neutral credential a prober (or client
// factory) needs to reach a cluster: an API server URL, an embedded CA, and one
// of a bearer token or a client cert/key pair. It carries no k8s or cloud SDK
// type, so the app layer can pass it without importing an adapter.
type ClusterCredential struct {
	APIURL     string
	CACert     []byte
	Token      string
	ClientCert []byte
	ClientKey  []byte
}

// ProbeFailureKind classifies why a cluster probe failed, so callers can map it
// to a status and message without string-matching.
type ProbeFailureKind string

const (
	// ProbeUnreachable means the API server could not be reached (network,
	// DNS, TLS, or an unexpected server error).
	ProbeUnreachable ProbeFailureKind = "unreachable"
	// ProbeUnauthorized means the credential was rejected (401).
	ProbeUnauthorized ProbeFailureKind = "unauthorized"
	// ProbeUnderPrivileged means the cluster is reachable and authenticated
	// but missing one of the least-privilege verbs the scheduler needs in the
	// namespace.
	ProbeUnderPrivileged ProbeFailureKind = "under_privileged"
)

// ProbeError is the typed, message-bearing error a ClusterProber returns.
// Callers inspect Kind (via errors.As) to decide how to surface it.
type ProbeError struct {
	Kind    ProbeFailureKind
	Message string
	Err     error
}

func (e *ProbeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("cluster %s: %s: %v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("cluster %s: %s", e.Kind, e.Message)
}

func (e *ProbeError) Unwrap() error { return e.Err }

// ClusterProber validates that a cluster is reachable, authenticated, and
// grants the least-privilege verbs Honryu needs in namespace, before a cluster
// is registered -- so an unusable cluster is rejected with a stated reason
// rather than discovered when a run fails. It returns a *ProbeError on failure.
type ClusterProber interface {
	Probe(ctx context.Context, cred ClusterCredential, namespace string) error
}
