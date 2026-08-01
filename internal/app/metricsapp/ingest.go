package metricsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/heridotlife/honryu/internal/domain/engine"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
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

	progressBatch := ports.ProgressBatch{
		RunID: runID, ShardIndex: batch.ShardIndex, StreamID: batch.StreamID,
		Final: batch.Final, ExitCode: batch.ExitCode, Intervals: batch.Intervals,
	}
	// Validated before anything is forwarded: a batch this malformed can only
	// come from a sidecar older than the control plane, and rejecting it after
	// the live view already published it would leave that view showing data the
	// permanent record refused.
	if err := progressBatch.Validate(); err != nil {
		return err
	}

	for _, in := range batch.Intervals {
		key := intervalKey(batch, in)
		if !s.seen.mark(batch.ExecutionID, key) {
			// Already absorbed; a retry of a batch that did arrive.
			continue
		}
		s.record(batch, in, runID)
	}

	// The permanent record is accumulated independently of the live-view dedup
	// above: ReportProgress keeps its own per-shard sequence, exact across a
	// control-plane restart in a way the in-memory seen map is not.
	if err := s.progress.Absorb(ctx, progressBatch); err != nil {
		return err
	}

	if !batch.Final {
		return nil
	}
	done, err := s.allShardsFinished(ctx, batch.ExecutionID, runID)
	if err != nil {
		return err
	}
	if !done {
		return nil
	}
	if err := s.finalizeCompleted(ctx, batch.ExecutionID, runID); err != nil {
		return err
	}
	// The run is over and its report is written; its intervals cannot arrive
	// again, so stop remembering them.
	s.seen.forget(batch.ExecutionID)
	return nil
}

// allShardsFinished reports whether every shard the execution's load profile
// called for has said it will send no more, which is how a run is known to be
// complete without asking a cluster.
func (s *Service) allShardsFinished(ctx context.Context, executionID, runID int64) (bool, error) {
	states, err := s.progress.ShardStates(ctx, runID)
	if err != nil {
		return false, err
	}
	finished := 0
	for _, st := range states {
		if st.Finished {
			finished++
		}
	}
	profile, err := s.repo.LoadProfileFor(ctx, executionID)
	if err != nil {
		return false, err
	}
	planned := 0
	for _, e := range profile {
		planned += e.Engines
	}
	return planned > 0 && finished >= planned, nil
}

// finalizeCompleted rolls up every shard's exit code into the run's outcome and
// finalises it. Called only once every shard has finished on its own -- a run
// Honryu stopped itself is finalised as an abort instead, by Finalize.
func (s *Service) finalizeCompleted(ctx context.Context, executionID, runID int64) error {
	states, err := s.progress.ShardStates(ctx, runID)
	if err != nil {
		return err
	}
	var codes []int
	for _, st := range states {
		if st.ExitCode != nil {
			codes = append(codes, *st.ExitCode)
		}
	}
	return s.finalize(ctx, executionID, runID, taurus.CombineOutcomes(codes))
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
