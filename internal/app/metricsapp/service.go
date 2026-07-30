// Package metricsapp is the metric use-case: it stamps each measurement with
// its execution/scenario/engine/run identity and fans it to the MetricsSink
// (Prometheus) and the EventBus (SSE subscribers). Collection is tracked per
// execution and is started and stopped by the lifecycle.
//
// Measurements used to be pulled from each engine's agent. Under Taurus they
// are pushed by a sidecar in the engine pod, so this package now offers Record
// as the entry point ingest calls; until that lands (task 21) nothing produces
// measurements and no live metrics flow.
package metricsapp

import (
	"context"
	"strconv"
	"sync"

	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/ports"
)

// Repo is the persistence the collector reads to know what is running.
type Repo interface {
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
	CurrentRun(ctx context.Context, executionID int64) (int64, bool, error)
	RunningScenarios(ctx context.Context) ([]ports.RunningScenario, error)
}

// Service collects and fans out engine metrics.
type Service struct {
	repo  Repo
	sched ports.Scheduler
	sink  ports.MetricsSink
	bus   ports.EventBus

	mu      sync.Mutex
	cancels map[int64]*collectRun
}

// collectRun identifies one background execution so its own goroutine can
// clean up without clobbering a later Start for the same execution.
type collectRun struct{ cancel context.CancelFunc }

// NewService wires the collector.
func NewService(repo Repo, sched ports.Scheduler, sink ports.MetricsSink, bus ports.EventBus) *Service {
	return &Service{repo: repo, sched: sched, sink: sink, bus: bus, cancels: map[int64]*collectRun{}}
}

// Start begins background execution for a execution's current run. It is
// idempotent: a second Start while execution is active is a no-op.
func (s *Service) Start(executionID int64) {
	s.mu.Lock()
	if _, ok := s.cancels[executionID]; ok {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	token := &collectRun{cancel: cancel}
	s.cancels[executionID] = token
	s.mu.Unlock()

	go func() {
		_ = s.CollectExecution(ctx, executionID)
		s.mu.Lock()
		if s.cancels[executionID] == token { // still ours
			delete(s.cancels, executionID)
		}
		s.mu.Unlock()
	}()
}

// Stop cancels background execution for an execution.
func (s *Service) Stop(executionID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token, ok := s.cancels[executionID]; ok {
		token.cancel()
		delete(s.cancels, executionID)
	}
}

// Purge stops execution and drops the execution's metric series.
func (s *Service) Purge(executionID int64) {
	s.Stop(executionID)
	s.sink.DeleteExecution(executionID)
}

// Resume restarts execution for every execution with running scenarios, so a
// restarted controller re-establishes its metric streams.
func (s *Service) Resume(ctx context.Context) error {
	rps, err := s.repo.RunningScenarios(ctx)
	if err != nil {
		return err
	}
	seen := map[int64]struct{}{}
	for _, rp := range rps {
		if _, ok := seen[rp.ExecutionID]; ok {
			continue
		}
		seen[rp.ExecutionID] = struct{}{}
		s.Start(rp.ExecutionID)
	}
	return nil
}

// CollectExecution is retained as the lifecycle's hook for a execution's run.
// It resolves the current run and its scenarios, which the ingest path will need
// to attribute pushed measurements; with the pull loop gone it does no work of
// its own.
func (s *Service) CollectExecution(ctx context.Context, executionID int64) error {
	if _, ok, err := s.repo.CurrentRun(ctx, executionID); err != nil || !ok {
		return err
	}
	if _, err := s.repo.LoadProfileFor(ctx, executionID); err != nil {
		return err
	}
	return nil
}

// Record stamps one measurement with the identity of the engine that produced
// it and forwards it to the metrics sink and the event bus (which feeds SSE
// subscribers).
//
// Measurements used to be pulled from each engine's agent. Under Taurus they
// arrive the other way round -- a sidecar in the engine pod pushes them to the
// control plane -- so this is the seam the ingest endpoint calls (task 21).
// Until that lands nothing calls it, and no live metrics flow.
func (s *Service) Record(executionID, scenarioID int64, engineID int, runID int64, m engine.Metric) {
	m.ExecutionID = strconv.FormatInt(executionID, 10)
	m.ScenarioID = strconv.FormatInt(scenarioID, 10)
	m.EngineID = strconv.Itoa(engineID)
	m.RunID = strconv.FormatInt(runID, 10)
	s.sink.Record(m)
	s.bus.Publish(executionID, m)
}
