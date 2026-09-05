//go:build integration

package mysql_test

import (
	"context"
	"sync"
	"testing"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLIntervalRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunIntervalRepositoryContract(t, func(t *testing.T) repositorytest.IntervalStore {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// The series row's latency is a JSON column merged in Go under a row lock, for
// the same reason the working state's label rows are: shards flush the same
// second concurrently, and without the lock the second write would silently
// discard the first shard's buckets. This drives that concurrently against
// real MySQL, mirroring the working state's own race test.
func TestMySQLIntervalRepository_ConcurrentShardsDoNotLoseEachOthersLatency(t *testing.T) {
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
					Concurrency: 5, Samples: 10, Succeeded: 10,
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

	got, err := repo.ListIntervalsByRun(ctx, 1)
	if err != nil {
		t.Fatalf("ListIntervalsByRun: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("seconds = %+v, want one merged second", got)
	}
	sec := got[0]
	if sec.Samples != shards*10 {
		t.Errorf("samples = %d, want %d -- every shard's count must survive", sec.Samples, shards*10)
	}
	if sec.Concurrency != shards*5 {
		t.Errorf("concurrency = %d, want %d summed across shards", sec.Concurrency, shards*5)
	}
	// The assertion the race actually breaks: each shard wrote a bucket at a
	// distinct key, so every key surviving is direct evidence no shard's
	// histogram write was lost to a concurrent overwrite.
	if len(sec.Latency) != shards {
		t.Errorf("latency buckets = %v, want %d distinct buckets (one per shard) -- a concurrent write silently dropped at least one shard's histogram",
			sec.Latency, shards)
	}
}
