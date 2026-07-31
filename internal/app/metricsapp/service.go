// Package metricsapp is the metric use-case: it absorbs the measurements engine
// pods push, stamps each with the pod that produced it, and fans it to the
// MetricsSink (Prometheus) and the EventBus (SSE subscribers).
//
// Measurements used to be pulled: the controller opened a stream to every
// engine's agent, which meant tracking which executions were being collected,
// re-establishing those streams after a restart, and losing whatever a pod
// measured once it became unreachable. Under Taurus a sidecar in each pod pushes
// instead, so none of that machinery has anything to do -- an unreachable pod is
// simply one that has stopped sending, and what it sent already arrived.
//
// What remains is dropping an execution's series when it is purged, since
// nothing else would ever remove them.
package metricsapp

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/ports"
)

// Repo is the persistence the service reads to attribute a pushed batch.
type Repo interface {
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
	CurrentRun(ctx context.Context, executionID int64) (int64, bool, error)
}

// Service absorbs pushed measurements.
type Service struct {
	repo Repo
	sink ports.MetricsSink
	bus  ports.EventBus
	// seen deduplicates intervals a pod pushed more than once.
	seen *seen
}

// NewService wires the metric service.
func NewService(repo Repo, sink ports.MetricsSink, bus ports.EventBus) *Service {
	return &Service{repo: repo, sink: sink, bus: bus, seen: newSeen()}
}

// Purge drops an execution's metric series and forgets what it absorbed.
//
// Called when an execution's engines are removed. Without it a long-lived
// controller would hold a series for every execution it had ever run.
func (s *Service) Purge(executionID int64) {
	s.sink.DeleteExecution(executionID)
	s.seen.forget(executionID)
}
