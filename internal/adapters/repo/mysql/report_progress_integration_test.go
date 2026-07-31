//go:build integration

package mysql_test

import (
	"context"
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
		RunID: 1, ShardIndex: 0, StreamID: "s1",
		Intervals: []metrics.Interval{{Seq: 1, Timestamp: 1000, Label: "probe", Samples: 1}},
	}
	ops := map[string]func() error{
		"Absorb":         func() error { return repo.Absorb(ctx, batch) },
		"Snapshot":       func() error { _, e := repo.Snapshot(ctx, 1); return e },
		"ShardsFinished": func() error { _, e := repo.ShardsFinished(ctx, 1); return e },
		"Discard":        func() error { return repo.Discard(ctx, 1) },
	}
	for name, op := range ops {
		if err := op(); err == nil {
			t.Errorf("%s on a closed database returned no error", name)
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
		RunID: 1, ShardIndex: 0, StreamID: "s1",
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
		RunID: 1, ShardIndex: 0, StreamID: "s1",
		Intervals: []metrics.Interval{{
			Seq: 2, Timestamp: 1001, Label: "probe", Samples: 1, Failed: 1,
			Errors: []metrics.ErrorGroup{{Message: "Not Found", ResponseCode: "404", Count: 1}},
		}},
	}
	if err := repo.Absorb(ctx, second); err == nil {
		t.Error("Absorb merging against corrupt stored progress returned no error")
	}
}
