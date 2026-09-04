//go:build integration

package mysql_test

import (
	"context"
	"sync"
	"testing"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/reportprogresstest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLReportProgress_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	reportprogresstest.Run(t, func(t *testing.T) ports.ReportProgress {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// Working state is read on every push and every finalisation, long after the
// process that wrote a row is gone. These drive the DB-error branches of every
// method by closing the pool first.
func TestMySQLReportProgress_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	batch := ports.ProgressBatch{
		RunID: 1, ScenarioID: 1, ShardIndex: 0, StreamID: "s1",
		Intervals: []metrics.Interval{{Seq: 1, Timestamp: 1000, Label: "probe", Samples: 1}},
	}
	ops := map[string]func() error{
		"Absorb":             func() error { return repo.Absorb(ctx, batch) },
		"Snapshot":           func() error { _, e := repo.Snapshot(ctx, 1); return e },
		"ShardStates":        func() error { _, e := repo.ShardStates(ctx, 1); return e },
		"Discard":            func() error { return repo.Discard(ctx, 1) },
		"ListIntervalsByRun": func() error { _, e := repo.ListIntervalsByRun(ctx, 1); return e },
	}
	for name, op := range ops {
		if err := op(); err == nil {
			t.Errorf("%s on a closed database returned no error", name)
		}
	}
}

// Shards flush independently, so two of them absorbing the same label at
// nearly the same moment is routine, not exceptional. Without a lock spanning
// the read and the write, both transactions would read the same pre-update
// histogram, merge their own delta on top of it, and whichever commits last
// would silently discard the other's buckets -- corrupting the run's latency
// percentiles with no error raised anywhere. This drives that concurrently for
// real against MySQL rather than asserting on the SQL shape.
func TestMySQLReportProgress_ConcurrentShardsDoNotLoseEachOthersLatency(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	const shards = 8
	var wg sync.WaitGroup
	errs := make([]error, shards)
	var start sync.WaitGroup
	start.Add(1)
	for shard := range shards {
		wg.Add(1)
		go func(shard int) {
			defer wg.Done()
			start.Wait() // maximise the chance every goroutine races at once
			errs[shard] = repo.Absorb(ctx, ports.ProgressBatch{
				RunID: 1, ScenarioID: 1, ShardIndex: shard, StreamID: "s", Final: true,
				Intervals: []metrics.Interval{{
					Seq: 1, Timestamp: 1000, Label: "checkout",
					Samples: 10, Succeeded: 10,
					Latency: metrics.Histogram{float64(shard) + 0.01: 10},
				}},
			})
		}(shard)
	}
	start.Done()
	wg.Wait()

	for shard, err := range errs {
		if err != nil {
			t.Fatalf("Absorb shard %d: %v", shard, err)
		}
	}

	snap, err := repo.Snapshot(ctx, 1)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Labels) != 1 {
		t.Fatalf("labels = %+v, want one (checkout)", snap.Labels)
	}
	label := snap.Labels[0]
	if label.Samples != shards*10 {
		t.Errorf("samples = %d, want %d -- every shard's count must survive even if this bug is fixed", label.Samples, shards*10)
	}
	// This is the assertion the race actually breaks: each shard wrote a bucket
	// at a distinct key (shard+0.01), so every key surviving is direct evidence
	// no shard's histogram write was lost to a concurrent overwrite.
	if len(label.Latency) != shards {
		t.Errorf("latency buckets = %v, want %d distinct buckets (one per shard) -- a concurrent write silently dropped at least one shard's histogram",
			label.Latency, shards)
	}
}

// mergeLabels/mergeSignatures batch every distinct label or signature in one
// query each, rather than one query per label -- this proves the batched read
// and the batched upsert both keep each label's own samples/failed/latency
// separate rather than cross-contaminating them.
func TestMySQLReportProgress_AbsorbMergesSeveralDistinctLabelsInOneBatch(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	labels := []string{"checkout", "cart", "search", "profile", "logout"}
	var intervals []metrics.Interval
	for i, label := range labels {
		intervals = append(intervals, metrics.Interval{
			Seq: int64(i) + 1, Timestamp: 1000, Label: label,
			Samples: int64(i) + 1, Succeeded: int64(i) + 1,
			Latency: metrics.Histogram{float64(i) + 0.01: int64(i) + 1},
		})
	}
	if err := repo.Absorb(ctx, ports.ProgressBatch{
		RunID: 1, ScenarioID: 1, ShardIndex: 0, StreamID: "s", Final: true,
		Intervals: intervals,
	}); err != nil {
		t.Fatalf("Absorb: %v", err)
	}

	snap, err := repo.Snapshot(ctx, 1)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Labels) != len(labels) {
		t.Fatalf("labels = %+v, want %d distinct labels", snap.Labels, len(labels))
	}
	byLabel := make(map[string]int64, len(labels))
	for _, l := range snap.Labels {
		byLabel[l.Label] = l.Samples
	}
	for i, label := range labels {
		if got, want := byLabel[label], int64(i)+1; got != want {
			t.Errorf("label %q samples = %d, want %d -- a batched merge must not cross-contaminate labels", label, got, want)
		}
	}
}

// Two shards absorbing overlapping sets of several labels each, in different
// orders, must not deadlock: the batched lock order comes from MySQL's own
// index scan over the primary key, not from the order labels happen to
// appear in either shard's batch.
func TestMySQLReportProgress_ConcurrentOverlappingMultiLabelBatchesDoNotDeadlock(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	forward := []string{"a", "b", "c", "d", "e"}
	backward := []string{"e", "d", "c", "b", "a"}

	// Distinct shard indices, each sending exactly one batch: Absorb's own
	// per-shard sequence dedup means concurrent batches for the *same* shard
	// racing in an unpredictable order would make one falsely look like an
	// already-absorbed retry of the other -- a hazard of a contrived test, not
	// what this is after. Separate shards racing on the same label rows is the
	// real scenario, and it needs no such care.
	const shardsPerOrder = 10
	total := 2 * shardsPerOrder
	var wg sync.WaitGroup
	errs := make([]error, total)
	var start sync.WaitGroup
	start.Add(1)
	run := func(errIdx, shardIndex int, order []string) {
		defer wg.Done()
		start.Wait()
		var intervals []metrics.Interval
		for j, label := range order {
			intervals = append(intervals, metrics.Interval{
				Seq: int64(j) + 1, Timestamp: 1000, Label: label,
				Samples: 1, Succeeded: 1, Latency: metrics.Histogram{0.01: 1},
			})
		}
		errs[errIdx] = repo.Absorb(ctx, ports.ProgressBatch{
			RunID: 1, ScenarioID: 1, ShardIndex: shardIndex, StreamID: "s",
			Intervals: intervals,
		})
	}
	for i := 0; i < shardsPerOrder; i++ {
		wg.Add(2)
		go run(i, i, forward)
		go run(shardsPerOrder+i, shardsPerOrder+i, backward)
	}
	start.Done()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Absorb %d: %v (want no deadlock)", i, err)
		}
	}

	snap, err := repo.Snapshot(ctx, 1)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Labels) != len(forward) {
		t.Fatalf("labels = %+v, want %d", snap.Labels, len(forward))
	}
	for _, l := range snap.Labels {
		if l.Samples != int64(total) {
			t.Errorf("label %q samples = %d, want %d (one per shard)", l.Label, l.Samples, total)
		}
	}
}

// A batch that merges into a label or signature already holding stored progress
// takes the read-then-merge path, not the plain insert every other test here
// exercises. Corrupting what is stored proves that path is reached and handled
// rather than merely present.
func TestMySQLReportProgress_MergeErrorsOnCorruptStoredProgress(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	first := ports.ProgressBatch{
		RunID: 1, ScenarioID: 1, ShardIndex: 0, StreamID: "s1",
		Intervals: []metrics.Interval{{
			Seq: 1, Timestamp: 1000, Label: "probe", Samples: 1, Failed: 1,
			Errors: []metrics.ErrorGroup{{Message: "Not Found", ResponseCode: "404", Count: 1}},
		}},
	}
	if err := repo.Absorb(ctx, first); err != nil {
		t.Fatalf("Absorb: %v", err)
	}

	// Valid JSON, wrong shape: the column accepts it, but it will not unmarshal
	// into the Go type the merge path expects (a histogram, a string slice).
	if _, err := db.Exec(`UPDATE report_progress_label SET latency='"not-a-histogram"' WHERE run_id=1`); err != nil {
		t.Fatalf("corrupt label progress: %v", err)
	}
	if _, err := db.Exec(`UPDATE report_progress_signature SET exemplars='{"not":"an-array"}' WHERE run_id=1`); err != nil {
		t.Fatalf("corrupt signature progress: %v", err)
	}

	second := ports.ProgressBatch{
		RunID: 1, ScenarioID: 1, ShardIndex: 0, StreamID: "s1",
		Intervals: []metrics.Interval{{
			Seq: 2, Timestamp: 1001, Label: "probe", Samples: 1, Failed: 1,
			Errors: []metrics.ErrorGroup{{Message: "Not Found", ResponseCode: "404", Count: 1}},
		}},
	}
	if err := repo.Absorb(ctx, second); err == nil {
		t.Error("Absorb merging against corrupt stored progress returned no error")
	}
}
