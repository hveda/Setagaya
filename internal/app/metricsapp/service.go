// Package metricsapp is the metric-execution use-case: it reads the live
// metric stream from every engine of a execution's running scenarios (via the
// Executor), stamps each measurement with its execution/scenario/engine/run
// identity, and fans it to the MetricsSink (Prometheus) and the EventBus (SSE
// subscribers). Collecting runs in the background per execution and is started
// and stopped by the lifecycle.
package metricsapp

import (
	"context"
	"strconv"
	"sync"

	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/ports"
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
	exec  ports.Executor
	sink  ports.MetricsSink
	bus   ports.EventBus

	mu      sync.Mutex
	cancels map[int64]*collectRun
}

// collectRun identifies one background execution so its own goroutine can
// clean up without clobbering a later Start for the same execution.
type collectRun struct{ cancel context.CancelFunc }

// NewService wires the collector.
func NewService(repo Repo, sched ports.Scheduler, exec ports.Executor, sink ports.MetricsSink, bus ports.EventBus) *Service {
	return &Service{repo: repo, sched: sched, exec: exec, sink: sink, bus: bus, cancels: map[int64]*collectRun{}}
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

// CollectExecution streams metrics for every scenario of a execution's current
// run until ctx is cancelled or all engine streams end.
func (s *Service) CollectExecution(ctx context.Context, executionID int64) error {
	runID, ok, err := s.repo.CurrentRun(ctx, executionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	scenarios, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, ep := range scenarios {
		wg.Add(1)
		go func(ep loadprofile.Entry) {
			defer wg.Done()
			if err := s.CollectScenario(ctx, executionID, ep.ScenarioID, ep.Engines, runID); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(ep)
	}
	wg.Wait()
	return firstErr
}

// CollectScenario streams metrics from every engine of a scenario.
func (s *Service) CollectScenario(ctx context.Context, executionID, scenarioID int64, engines int, runID int64) error {
	urls, err := s.sched.EngineURLs(ctx, executionID, scenarioID, engines)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(engineID int, url string) {
			defer wg.Done()
			s.pumpEngine(ctx, executionID, scenarioID, engineID, runID, url)
		}(i, url)
	}
	wg.Wait()
	return nil
}

// pumpEngine subscribes to one engine and forwards every measurement, stamped
// with its identity, to the sink and the bus.
func (s *Service) pumpEngine(ctx context.Context, executionID, scenarioID int64, engineID int, runID int64, url string) {
	ch, err := s.exec.Subscribe(ctx, url)
	if err != nil {
		return
	}
	collStr := strconv.FormatInt(executionID, 10)
	planStr := strconv.FormatInt(scenarioID, 10)
	engStr := strconv.Itoa(engineID)
	runStr := strconv.FormatInt(runID, 10)
	for m := range ch {
		m.ExecutionID = collStr
		m.ScenarioID = planStr
		m.EngineID = engStr
		m.RunID = runStr
		s.sink.Record(m)
		s.bus.Publish(executionID, m)
	}
}
