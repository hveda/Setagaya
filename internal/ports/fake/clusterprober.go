package fake

import (
	"context"

	"github.com/heridotlife/honryu/internal/ports"
)

// ClusterProber is a fake ports.ClusterProber that returns a pre-set outcome,
// letting app-layer tests inject a reachable / unreachable / under-privileged
// result without a real cluster. Calls records what it was asked to probe.
type ClusterProber struct {
	Err   error
	Calls []ClusterProbeCall
}

// ClusterProbeCall records one Probe invocation.
type ClusterProbeCall struct {
	Cred      ports.ClusterCredential
	Namespace string
}

var _ ports.ClusterProber = (*ClusterProber)(nil)

// Probe records the call and returns the configured Err.
func (p *ClusterProber) Probe(_ context.Context, cred ports.ClusterCredential, namespace string) error {
	p.Calls = append(p.Calls, ClusterProbeCall{Cred: cred, Namespace: namespace})
	return p.Err
}
