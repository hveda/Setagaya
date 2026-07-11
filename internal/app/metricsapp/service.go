// Package metricsapp is the metric-collection use-case: it reads the live
// metric stream from every engine of a collection's running plans (via the
// Executor), stamps each measurement with its collection/plan/engine/run
// identity, and fans it to the MetricsSink (Prometheus) and the EventBus (SSE
// subscribers). Collection runs in the background per collection and is started
// and stopped by the lifecycle.
package metricsapp

import (
	"context"
	"strconv"
	"sync"

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Repo is the persistence the collector reads to know what is running.
type Repo interface {
	ExecutionPlansFor(ctx context.Context, collectionID int64) ([]execution.ExecutionPlan, error)
	CurrentRun(ctx context.Context, collectionID int64) (int64, bool, error)
	RunningPlans(ctx context.Context) ([]ports.RunningPlan, error)
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

// collectRun identifies one background collection so its own goroutine can
// clean up without clobbering a later Start for the same collection.
type collectRun struct{ cancel context.CancelFunc }

// NewService wires the collector.
func NewService(repo Repo, sched ports.Scheduler, exec ports.Executor, sink ports.MetricsSink, bus ports.EventBus) *Service {
	return &Service{repo: repo, sched: sched, exec: exec, sink: sink, bus: bus, cancels: map[int64]*collectRun{}}
}

// Start begins background collection for a collection's current run. It is
// idempotent: a second Start while collection is active is a no-op.
func (s *Service) Start(collectionID int64) {
	s.mu.Lock()
	if _, ok := s.cancels[collectionID]; ok {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	token := &collectRun{cancel: cancel}
	s.cancels[collectionID] = token
	s.mu.Unlock()

	go func() {
		_ = s.CollectCollection(ctx, collectionID)
		s.mu.Lock()
		if s.cancels[collectionID] == token { // still ours
			delete(s.cancels, collectionID)
		}
		s.mu.Unlock()
	}()
}

// Stop cancels background collection for a collection.
func (s *Service) Stop(collectionID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token, ok := s.cancels[collectionID]; ok {
		token.cancel()
		delete(s.cancels, collectionID)
	}
}

// Purge stops collection and drops the collection's metric series.
func (s *Service) Purge(collectionID int64) {
	s.Stop(collectionID)
	s.sink.DeleteCollection(collectionID)
}

// Resume restarts collection for every collection with running plans, so a
// restarted controller re-establishes its metric streams.
func (s *Service) Resume(ctx context.Context) error {
	rps, err := s.repo.RunningPlans(ctx)
	if err != nil {
		return err
	}
	seen := map[int64]struct{}{}
	for _, rp := range rps {
		if _, ok := seen[rp.CollectionID]; ok {
			continue
		}
		seen[rp.CollectionID] = struct{}{}
		s.Start(rp.CollectionID)
	}
	return nil
}

// CollectCollection streams metrics for every plan of a collection's current
// run until ctx is cancelled or all engine streams end.
func (s *Service) CollectCollection(ctx context.Context, collectionID int64) error {
	runID, ok, err := s.repo.CurrentRun(ctx, collectionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	plans, err := s.repo.ExecutionPlansFor(ctx, collectionID)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, ep := range plans {
		wg.Add(1)
		go func(ep execution.ExecutionPlan) {
			defer wg.Done()
			if err := s.CollectPlan(ctx, collectionID, ep.PlanID, ep.Engines, runID); err != nil {
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

// CollectPlan streams metrics from every engine of a plan.
func (s *Service) CollectPlan(ctx context.Context, collectionID, planID int64, engines int, runID int64) error {
	urls, err := s.sched.EngineURLs(ctx, collectionID, planID, engines)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(engineID int, url string) {
			defer wg.Done()
			s.pumpEngine(ctx, collectionID, planID, engineID, runID, url)
		}(i, url)
	}
	wg.Wait()
	return nil
}

// pumpEngine subscribes to one engine and forwards every measurement, stamped
// with its identity, to the sink and the bus.
func (s *Service) pumpEngine(ctx context.Context, collectionID, planID int64, engineID int, runID int64, url string) {
	ch, err := s.exec.Subscribe(ctx, url)
	if err != nil {
		return
	}
	collStr := strconv.FormatInt(collectionID, 10)
	planStr := strconv.FormatInt(planID, 10)
	engStr := strconv.Itoa(engineID)
	runStr := strconv.FormatInt(runID, 10)
	for m := range ch {
		m.CollectionID = collStr
		m.PlanID = planStr
		m.EngineID = engStr
		m.RunID = runStr
		s.sink.Record(m)
		s.bus.Publish(collectionID, m)
	}
}
