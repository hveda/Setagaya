// Package reportprogresstest is the shared conformance suite every
// ReportProgress must pass, fake and real alike.
package reportprogresstest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewProgress builds a store with no runs in it.
type NewProgress func(t *testing.T) ports.ReportProgress

// Run exercises ReportProgress behaviour.
func Run(t *testing.T, newProgress NewProgress) {
	t.Helper()
	ctx := context.Background()

	t.Run("AbsorbAccumulates", func(t *testing.T) {
		p := newProgress(t)
		if err := p.Absorb(ctx, batch(1, 0, "s1", false,
			iv(1, 1000, "probe", 10, 5, 0),
			iv(2, 1001, "probe", 10, 5, 0),
		)); err != nil {
			t.Fatalf("Absorb: %v", err)
		}

		got := restore(t, p, 1)
		if len(got.Labels) != 1 || got.Labels[0].Samples != 20 {
			t.Errorf("labels = %+v, want one with 20 samples", got.Labels)
		}
	})

	// The reason intervals carry a sequence. A push whose response was lost is
	// followed by a superset batch: the intervals that did arrive plus everything
	// read since. The old ones must not be counted twice, and the new ones must
	// not be lost with them.
	t.Run("RetryOfASupersetBatchCountsEachIntervalOnce", func(t *testing.T) {
		p := newProgress(t)
		first := []metrics.Interval{iv(1, 1000, "probe", 10, 5, 0)}
		if err := p.Absorb(ctx, batch(1, 0, "s1", false, first...)); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		// The sidecar never saw the acknowledgement, so it re-sends the first
		// interval alongside one read since.
		if err := p.Absorb(ctx, batch(1, 0, "s1", false,
			iv(1, 1000, "probe", 10, 5, 0),
			iv(2, 1001, "probe", 7, 3, 0),
		)); err != nil {
			t.Fatalf("Absorb retry: %v", err)
		}

		got := restore(t, p, 1)
		if len(got.Labels) != 1 {
			t.Fatalf("labels = %+v, want one", got.Labels)
		}
		if got.Labels[0].Samples != 17 {
			t.Errorf("samples = %d, want 17 -- the retried interval counted once, the new one kept",
				got.Labels[0].Samples)
		}
	})

	// A batch boundary can fall between two labels of the same second, so a
	// second arriving in two pieces must accumulate to the whole.
	t.Run("ASecondSplitAcrossBatches", func(t *testing.T) {
		p := newProgress(t)
		if err := p.Absorb(ctx, batch(1, 0, "s1", false, iv(1, 1000, "cart", 10, 0, 4))); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		if err := p.Absorb(ctx, batch(1, 0, "s1", false, iv(2, 1000, "pay", 6, 0, 3))); err != nil {
			t.Fatalf("Absorb: %v", err)
		}

		got := restore(t, p, 1)
		if len(got.Labels) != 2 {
			t.Fatalf("labels = %+v, want both halves of the second", got.Labels)
		}
		var second report.SecondProgress
		for _, s := range got.Seconds {
			if s.Second == 1000 {
				second = s
			}
		}
		if second.Labels != 7 {
			t.Errorf("concurrency at 1000 = %d, want 7 summed across both labels", second.Labels)
		}
	})

	// Shards contribute independently and additively.
	t.Run("ShardsAccumulateTogether", func(t *testing.T) {
		p := newProgress(t)
		for shard := range 3 {
			if err := p.Absorb(ctx, batch(1, shard, "s1", false,
				iv(1, 1000, "probe", 10, 1, 5),
			)); err != nil {
				t.Fatalf("Absorb shard %d: %v", shard, err)
			}
		}
		got := restore(t, p, 1)
		if got.Labels[0].Samples != 30 {
			t.Errorf("samples = %d, want 30 across three shards", got.Labels[0].Samples)
		}
		// Each shard's sequence is its own: three shards at seq 1 are three
		// intervals, not one interval sent thrice.
		if got.Seconds[0].Labels != 15 {
			t.Errorf("concurrency = %d, want 15 summed across shards", got.Seconds[0].Labels)
		}
	})

	// A restarted pod begins its sequence again at one. Treating that as a
	// duplicate would discard everything it measures for the rest of the run.
	t.Run("ARestartedShardIsNotMistakenForADuplicate", func(t *testing.T) {
		p := newProgress(t)
		if err := p.Absorb(ctx, batch(1, 0, "s1", false,
			iv(1, 1000, "probe", 10, 0, 0),
			iv(2, 1001, "probe", 10, 0, 0),
		)); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		// The pod restarted: new stream, sequence back to one.
		if err := p.Absorb(ctx, batch(1, 0, "s2", false,
			iv(1, 1002, "probe", 8, 0, 0),
		)); err != nil {
			t.Fatalf("Absorb after restart: %v", err)
		}

		got := restore(t, p, 1)
		if got.Labels[0].Samples != 28 {
			t.Errorf("samples = %d, want 28 -- the restarted stream was discarded", got.Labels[0].Samples)
		}
	})

	// A batch that cannot be deduplicated is refused rather than absorbed.
	t.Run("RejectsUnsequencedIntervals", func(t *testing.T) {
		p := newProgress(t)
		unsequenced := batch(1, 0, "s1", false, iv(0, 1000, "probe", 10, 0, 0))
		if err := p.Absorb(ctx, unsequenced); !errors.Is(err, ports.ErrUnsequencedBatch) {
			t.Errorf("Absorb(unsequenced) = %v, want ErrUnsequencedBatch", err)
		}
		got := restore(t, p, 1)
		if len(got.Labels) != 0 {
			t.Errorf("a rejected batch was absorbed anyway: %+v", got.Labels)
		}
	})

	t.Run("CountsFinishedShards", func(t *testing.T) {
		p := newProgress(t)
		for shard := range 3 {
			final := shard < 2
			if err := p.Absorb(ctx, batch(1, shard, "s1", final, iv(1, 1000, "probe", 5, 0, 0))); err != nil {
				t.Fatalf("Absorb: %v", err)
			}
		}
		n, err := p.ShardsFinished(ctx, 1)
		if err != nil {
			t.Fatalf("ShardsFinished: %v", err)
		}
		if n != 2 {
			t.Errorf("finished shards = %d, want 2", n)
		}
	})

	t.Run("SnapshotOfAnUnknownRunIsEmpty", func(t *testing.T) {
		p := newProgress(t)
		got, err := p.Snapshot(ctx, 999)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if len(got.Labels) != 0 || len(got.Seconds) != 0 || len(got.Signatures) != 0 {
			t.Errorf("snapshot of an unknown run = %+v", got)
		}
		n, err := p.ShardsFinished(ctx, 999)
		if err != nil || n != 0 {
			t.Errorf("ShardsFinished(unknown) = %d, %v", n, err)
		}
	})

	// Working state is discarded once it has become a report, and discarding a
	// run that has none is what a re-run of finalisation does.
	t.Run("DiscardIsIdempotent", func(t *testing.T) {
		p := newProgress(t)
		if err := p.Absorb(ctx, batch(1, 0, "s1", true, iv(1, 1000, "probe", 10, 0, 0))); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		if err := p.Discard(ctx, 1); err != nil {
			t.Fatalf("Discard: %v", err)
		}
		if err := p.Discard(ctx, 1); err != nil {
			t.Errorf("Discard of an already-discarded run: %v", err)
		}
		got := restore(t, p, 1)
		if len(got.Labels) != 0 {
			t.Errorf("state survived Discard: %+v", got.Labels)
		}
		if n, _ := p.ShardsFinished(ctx, 1); n != 0 {
			t.Errorf("finished shards after Discard = %d, want 0", n)
		}
	})

	// Runs must not bleed into each other: two runs accumulating at once is the
	// normal state of a busy control plane.
	t.Run("RunsAreIsolated", func(t *testing.T) {
		p := newProgress(t)
		if err := p.Absorb(ctx, batch(1, 0, "s1", false, iv(1, 1000, "probe", 10, 0, 0))); err != nil {
			t.Fatalf("Absorb run 1: %v", err)
		}
		if err := p.Absorb(ctx, batch(2, 0, "s1", false, iv(1, 1000, "probe", 99, 0, 0))); err != nil {
			t.Fatalf("Absorb run 2: %v", err)
		}
		if got := restore(t, p, 1).Labels[0].Samples; got != 10 {
			t.Errorf("run 1 samples = %d, want 10", got)
		}
		if got := restore(t, p, 2).Labels[0].Samples; got != 99 {
			t.Errorf("run 2 samples = %d, want 99", got)
		}
	})

	// The whole point of persisting the working state: what is read back has to
	// produce the same report as the accumulator that wrote it.
	t.Run("SnapshotRebuildsTheSameReport", func(t *testing.T) {
		p := newProgress(t)
		intervals := []metrics.Interval{
			iv(1, 1000, "cart", 100, 10, 20),
			iv(2, 1000, "pay", 50, 5, 10),
			iv(3, 1001, "cart", 100, 0, 20),
		}
		intervals[0].Errors = []metrics.ErrorGroup{
			{Message: "Not Found", ResponseCode: "404", Count: 10},
		}
		intervals[1].Errors = []metrics.ErrorGroup{
			{Message: "socket: too many open files", Count: 5},
		}
		if err := p.Absorb(ctx, batch(1, 0, "s1", true, intervals...)); err != nil {
			t.Fatalf("Absorb: %v", err)
		}

		meta := report.Meta{ExecutionID: 1, RunID: 1, Outcome: "failed",
			Requested: report.Load{DurationSeconds: 10}}
		direct := report.NewAccumulator()
		for _, in := range intervals {
			direct.Add(in)
		}
		if got, want := restoreAcc(t, p, 1).Report(meta), direct.Report(meta); !reflect.DeepEqual(got, want) {
			t.Errorf("report from stored state differs:\n got %+v\nwant %+v", got, want)
		}
	})
}

func batch(runID int64, shard int, stream string, final bool, intervals ...metrics.Interval) ports.ProgressBatch {
	return ports.ProgressBatch{
		RunID: runID, ShardIndex: shard, StreamID: stream, Final: final, Intervals: intervals,
	}
}

func iv(seq, ts int64, label string, samples, failed int64, concurrency int) metrics.Interval {
	return metrics.Interval{
		Seq: seq, Timestamp: ts, Label: label,
		Samples: samples, Failed: failed, Succeeded: samples - failed,
		Concurrency: concurrency,
		Latency:     metrics.Histogram{0.01: samples},
	}
}

func restore(t *testing.T, p ports.ReportProgress, runID int64) report.Snapshot {
	t.Helper()
	got, err := p.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return got
}

func restoreAcc(t *testing.T, p ports.ReportProgress, runID int64) *report.Accumulator {
	t.Helper()
	return report.Restore(restore(t, p, runID))
}
