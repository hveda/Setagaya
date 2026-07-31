package metricsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/domain/metrics"
)

// Ingest errors. Callers compare with errors.Is.
var (
	// ErrNoActiveRun means the execution is not running. A pod that outlived its
	// run must not contribute to whatever runs next.
	ErrNoActiveRun = errors.New("metricsapp: execution has no active run")
	// ErrStaleRun means the batch belongs to a run that has since ended.
	ErrStaleRun = errors.New("metricsapp: batch belongs to a finished run")
)

// seen remembers which intervals a run has already absorbed, so a batch that
// arrives twice is counted once.
//
// The sidecar keeps a failed push pending and retries it, which is what makes
// a brief control-plane outage survivable -- and what guarantees duplicates.
// Without this, a retried batch would double every counter it carried.
type seen struct {
	mu   sync.Mutex
	runs map[int64]map[string]struct{} // executionID -> interval key
}

func newSeen() *seen {
	return &seen{runs: map[int64]map[string]struct{}{}}
}

// mark records an interval and reports whether it is new.
func (s *seen) mark(executionID int64, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys, ok := s.runs[executionID]
	if !ok {
		keys = map[string]struct{}{}
		s.runs[executionID] = keys
	}
	if _, dup := keys[key]; dup {
		return false
	}
	keys[key] = struct{}{}
	return true
}

// forget drops an execution's history once its run is over, so a long-lived
// controller does not accumulate every interval it has ever seen.
func (s *seen) forget(executionID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, executionID)
}

// Ingest absorbs one engine pod's measurements.
//
// Batches arrive by push from a sidecar inside the pod. Nothing here reaches
// back into a cluster, which is what lets an execution run somewhere the control
// plane cannot address.
//
// Shards contribute independently and additively, so a pod that dies mid-run
// simply stops contributing: the aggregate stays valid, describing the load that
// was actually produced rather than becoming corrupt.
func (s *Service) Ingest(ctx context.Context, batch metrics.Batch) error {
	runID, running, err := s.repo.CurrentRun(ctx, batch.ExecutionID)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("%w: execution %d", ErrNoActiveRun, batch.ExecutionID)
	}
	// A pod from an earlier run must not pollute the current one. This is the
	// case that matters after a re-deploy: the old pods are still dying while
	// the new ones start.
	if batch.RunID != 0 && batch.RunID != runID {
		return fmt.Errorf("%w: batch run %d, current run %d", ErrStaleRun, batch.RunID, runID)
	}

	for _, in := range batch.Intervals {
		key := intervalKey(batch, in)
		if !s.seen.mark(batch.ExecutionID, key) {
			// Already absorbed; a retry of a batch that did arrive.
			continue
		}
		s.record(batch, in, runID)
	}

	if batch.Final {
		// The pod is done. Its intervals cannot arrive again, so stop
		// remembering them.
		s.seen.forget(batch.ExecutionID)
	}
	return nil
}

// intervalKey identifies one measurement uniquely within a run: which pod, which
// second, which request.
func intervalKey(b metrics.Batch, in metrics.Interval) string {
	return strconv.Itoa(b.ShardIndex) + "|" +
		strconv.FormatInt(b.ScenarioID, 10) + "|" +
		strconv.FormatInt(in.Timestamp, 10) + "|" + in.Label
}

// record forwards one interval to the metrics sink and the event bus.
//
// The sink and the SSE stream still speak the per-measurement shape the agent
// protocol used, so an interval is expressed in those terms: its average latency
// stands in for the samples it summarises. The buckets it carries are what
// matter for percentiles, and those are aggregated separately.
func (s *Service) record(b metrics.Batch, in metrics.Interval, runID int64) {
	status := "200"
	if in.Failed > 0 && in.Succeeded == 0 {
		status = "500"
	}
	m := engine.Metric{
		Label:       in.Label,
		Latency:     in.Latency.Percentile(50),
		Threads:     float64(in.Concurrency),
		Status:      status,
		ExecutionID: strconv.FormatInt(b.ExecutionID, 10),
		ScenarioID:  strconv.FormatInt(b.ScenarioID, 10),
		EngineID:    strconv.Itoa(b.ShardIndex),
		RunID:       strconv.FormatInt(runID, 10),
	}
	s.sink.Record(m)
	s.bus.Publish(b.ExecutionID, m)
}
