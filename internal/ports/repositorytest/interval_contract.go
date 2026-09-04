package repositorytest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/ports"
)

// IntervalStore is what the interval contract exercises. The series rows are
// written by Absorb -- in the real adapter, inside its very transaction, the
// shard watermark filtering a retry's re-sends before a row is ever written --
// so a store under test must be both halves of that arrangement at once.
type IntervalStore interface {
	ports.ReportProgress
	ports.IntervalRepository
}

// NewIntervalStore builds a fresh, empty IntervalStore for one test.
type NewIntervalStore func(t *testing.T) IntervalStore

// RunIntervalRepositoryContract pins the behaviour every IntervalStore must
// share, fake and real alike: what Absorb writes into the series, that the
// watermark keeps retries out of it, and that finalisation does not erase it.
func RunIntervalRepositoryContract(t *testing.T, newStore NewIntervalStore) {
	t.Helper()

	ctx := context.Background()

	// The whole point of the table: unlike the working state, the series is
	// the report's evidence and must survive finalisation's Discard.
	t.Run("SeriesSurvivesFinalisation", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false, seriesRow(1, 1000, "cart", 5, 10, 1))); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		if err := s.Discard(ctx, 1); err != nil {
			t.Fatalf("Discard: %v", err)
		}
		got, err := s.ListIntervalsByRun(ctx, 1)
		if err != nil {
			t.Fatalf("ListIntervalsByRun after Discard: %v", err)
		}
		if len(got) != 1 || got[0].Timestamp != 1000 || got[0].Samples != 10 {
			t.Fatalf("series after finalisation = %+v, want the measured second to survive", got)
		}
	})

	// The reason intervals carry a sequence, asserted where its effect lands:
	// a push whose response was lost is followed by a superset batch, and the
	// re-sent intervals must not extend the series a second time.
	t.Run("RetryOfASupersetBatchCountsEachSecondOnce", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false, seriesRow(1, 1000, "cart", 5, 10, 1))); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false,
			seriesRow(1, 1000, "cart", 5, 10, 1), // re-sent: the ack was lost
			seriesRow(2, 1001, "cart", 5, 8, 0),  // read since the lost push
		)); err != nil {
			t.Fatalf("Absorb retry: %v", err)
		}

		got := listOrFail(t, s, 1)
		if len(got) != 2 {
			t.Fatalf("seconds = %+v, want 1000 and 1001 only", got)
		}
		if got[0].Samples != 10 || got[1].Samples != 8 {
			t.Fatalf("samples = [%d, %d], want [10, 8] -- the retried second counted once", got[0].Samples, got[1].Samples)
		}
	})

	// Shards flush independently, so the same second arriving from several of
	// them is the normal case, not an exception: counts sum, buckets union,
	// and virtual users are the sum of what each shard held that second.
	t.Run("ShardsSumIntoTheSameSecond", func(t *testing.T) {
		s := newStore(t)
		for shard := range 2 {
			if err := s.Absorb(ctx, seriesBatch(1, shard, "s1", false, metrics.Interval{
				Seq: 1, Timestamp: 1000, Label: "cart", Concurrency: 5,
				Samples: 10, Succeeded: 9, Failed: 1,
				Latency: metrics.Histogram{float64(shard) + 0.01: 10},
			})); err != nil {
				t.Fatalf("Absorb shard %d: %v", shard, err)
			}
		}

		got := listOrFail(t, s, 1)
		if len(got) != 1 {
			t.Fatalf("seconds = %+v, want one merged second", got)
		}
		sec := got[0]
		if sec.Samples != 20 || sec.Failed != 2 || sec.Concurrency != 10 {
			t.Errorf("second = %+v, want samples 20, failed 2, concurrency 10 (summed across shards)", sec)
		}
		if len(sec.Latency) != 2 {
			t.Errorf("latency = %v, want both shards' distinct buckets unioned", sec.Latency)
		}
	})

	// The engine's own __total__ row re-counts the label rows' samples, so it
	// must contribute concurrency only -- and where its reading of virtual
	// users is the higher one, it is the one a chart must see.
	t.Run("EngineTotalRowCountsConcurrencyOnly", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false,
			seriesRow(1, 1000, "cart", 5, 10, 0),
			metrics.Interval{Seq: 2, Timestamp: 1000, Label: report.TotalLabel, Concurrency: 7, Samples: 10, Succeeded: 10},
		)); err != nil {
			t.Fatalf("Absorb: %v", err)
		}

		got := listOrFail(t, s, 1)
		if len(got) != 1 {
			t.Fatalf("seconds = %+v, want one", got)
		}
		if got[0].Samples != 10 {
			t.Errorf("samples = %d, want 10 -- the total row re-counts the labels and must not double them", got[0].Samples)
		}
		if got[0].Concurrency != 7 {
			t.Errorf("concurrency = %d, want 7 -- the engine's own reading wins where it is higher", got[0].Concurrency)
		}
	})

	// A restarted pod begins its sequence again at one, and re-measures
	// seconds the run already charted: additive merging keeps both passes,
	// exactly as the working state's accumulator keeps them.
	t.Run("RestartedStreamMergesBothPasses", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false, seriesRow(1, 1000, "cart", 5, 10, 0))); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s2", false, seriesRow(1, 1000, "cart", 3, 4, 0))); err != nil {
			t.Fatalf("Absorb after restart: %v", err)
		}

		got := listOrFail(t, s, 1)
		if len(got) != 1 || got[0].Samples != 14 {
			t.Fatalf("second = %+v, want samples 14 -- the restarted stream's pass was discarded", got)
		}
	})

	t.Run("SecondsWithoutSamplesCarryNoLatency", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false,
			metrics.Interval{Seq: 1, Timestamp: 1000, Label: "cart", Concurrency: 4},
		)); err != nil {
			t.Fatalf("Absorb: %v", err)
		}
		got := listOrFail(t, s, 1)
		if got[0].Latency != nil {
			t.Errorf("latency = %v, want unset for a second with no samples", got[0].Latency)
		}
	})

	t.Run("OrderedBySecondAcrossBatches", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false,
			seriesRow(2, 1001, "cart", 5, 8, 0),
			seriesRow(3, 1002, "cart", 5, 9, 0),
		)); err != nil {
			t.Fatalf("Absorb (later seconds): %v", err)
		}
		// A different shard, whose own sequence starts at one: within one
		// stream seconds arrive in sequence order, so an earlier second
		// reaching the store late comes from another pod.
		if err := s.Absorb(ctx, seriesBatch(1, 1, "s1", false, seriesRow(1, 1000, "cart", 5, 10, 0))); err != nil {
			t.Fatalf("Absorb (earlier second): %v", err)
		}
		got := listOrFail(t, s, 1)
		if len(got) != 3 || got[0].Timestamp >= got[1].Timestamp || got[1].Timestamp >= got[2].Timestamp {
			t.Fatalf("seconds = %+v, want ascending timestamps however the batches arrived", got)
		}
	})

	t.Run("UnknownRunIsEmptyNotAnError", func(t *testing.T) {
		s := newStore(t)
		got, err := s.ListIntervalsByRun(ctx, 999)
		if err != nil {
			t.Fatalf("ListIntervalsByRun(unknown) = %v, want no error", err)
		}
		if len(got) != 0 {
			t.Fatalf("ListIntervalsByRun(unknown) = %+v, want empty", got)
		}
	})

	// A batch that cannot be deduplicated is refused by the progress
	// contract; this pins that the refusal leaves no series behind either;
	// which is where a partial write would silently corrupt a chart.
	t.Run("RefusedBatchLeavesNoSeriesBehind", func(t *testing.T) {
		s := newStore(t)
		bad := seriesBatch(1, 0, "s1", false,
			seriesRow(1, 1000, "cart", 5, 10, 0),
			metrics.Interval{Timestamp: 1001, Label: "cart", Samples: 3}, // no seq
		)
		if err := s.Absorb(ctx, bad); !errors.Is(err, ports.ErrUnsequencedBatch) {
			t.Fatalf("Absorb(unsequenced) = %v, want ErrUnsequencedBatch", err)
		}
		if got := listOrFail(t, s, 1); len(got) != 0 {
			t.Fatalf("a refused batch left series rows behind: %+v", got)
		}
	})

	t.Run("RunsAreIsolated", func(t *testing.T) {
		s := newStore(t)
		if err := s.Absorb(ctx, seriesBatch(1, 0, "s1", false, seriesRow(1, 1000, "cart", 5, 10, 0))); err != nil {
			t.Fatalf("Absorb run 1: %v", err)
		}
		if err := s.Absorb(ctx, seriesBatch(2, 0, "s1", false, seriesRow(1, 1000, "cart", 5, 99, 0))); err != nil {
			t.Fatalf("Absorb run 2: %v", err)
		}
		if got := listOrFail(t, s, 1); got[0].Samples != 10 {
			t.Errorf("run 1 samples = %d, want 10 (run 2 leaked in)", got[0].Samples)
		}
		if got := listOrFail(t, s, 2); got[0].Samples != 99 {
			t.Errorf("run 2 samples = %d, want 99", got[0].Samples)
		}
	})

	// The honest end-to-end property a results page relies on: the series a
	// run's own batches produce is identical whether any push was retried.
	t.Run("RetriedRunMatchesCleanRun", func(t *testing.T) {
		clean := newStore(t)
		if err := clean.Absorb(ctx, seriesBatch(7, 0, "s1", true,
			seriesRow(1, 1000, "cart", 5, 10, 1),
			seriesRow(2, 1001, "cart", 5, 10, 0),
		)); err != nil {
			t.Fatalf("Absorb clean: %v", err)
		}
		if err := clean.Absorb(ctx, seriesBatch(7, 1, "s1", true,
			seriesRow(1, 1000, "cart", 4, 8, 0),
			seriesRow(2, 1001, "cart", 4, 9, 1),
		)); err != nil {
			t.Fatalf("Absorb clean shard 1: %v", err)
		}

		retried := newStore(t)
		// First shard's push is lost after commit: re-sent as a superset.
		if err := retried.Absorb(ctx, retried7(0, seriesRow(1, 1000, "cart", 5, 10, 1))); err != nil {
			t.Fatalf("Absorb retried: %v", err)
		}
		if err := retried.Absorb(ctx, retried7(0,
			seriesRow(1, 1000, "cart", 5, 10, 1),
			seriesRow(2, 1001, "cart", 5, 10, 0),
		)); err != nil {
			t.Fatalf("Absorb retried superset: %v", err)
		}
		if err := retried.Absorb(ctx, retried7(1,
			seriesRow(1, 1000, "cart", 4, 8, 0),
			seriesRow(2, 1001, "cart", 4, 9, 1),
		)); err != nil {
			t.Fatalf("Absorb retried shard 1: %v", err)
		}

		want := listOrFail(t, clean, 7)
		got := listOrFail(t, retried, 7)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("retried run's series:\n got %+v\nwant %+v (the clean run's)", got, want)
		}
	})
}

func listOrFail(t *testing.T, s IntervalStore, runID int64) []metrics.Interval {
	t.Helper()
	got, err := s.ListIntervalsByRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("ListIntervalsByRun: %v", err)
	}
	return got
}

// seriesRow is one pod's row: buckets sized to its samples by default.
func seriesRow(seq, ts int64, label string, concurrency int, samples, failed int64) metrics.Interval {
	return metrics.Interval{
		Seq: seq, Timestamp: ts, Label: label, Concurrency: concurrency,
		Samples: samples, Succeeded: samples - failed, Failed: failed,
		Latency: metrics.Histogram{0.01: samples},
	}
}

// retried7 is seriesBatch pinned to run 7, whose retry case needs the run id
// three times and reads better with it named once.
func retried7(shard int, intervals ...metrics.Interval) ports.ProgressBatch {
	return seriesBatch(7, shard, "s1", false, intervals...)
}

func seriesBatch(runID int64, shard int, stream string, final bool, intervals ...metrics.Interval) ports.ProgressBatch {
	return ports.ProgressBatch{
		RunID: runID, ScenarioID: 1, ShardIndex: shard, StreamID: stream, Final: final, Intervals: intervals,
	}
}
