package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/ports"
)

var _ ports.IntervalRepository = (*Repository)(nil)

// ListIntervalsByRun returns a run's per-second measurements, ascending by
// second, as the series endpoint charts them.
func (r *Repository) ListIntervalsByRun(ctx context.Context, runID int64) ([]metrics.Interval, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT second, GREATEST(engine_concurrency, label_concurrency), samples, failed, bytes, latency
		 FROM execution_report_series WHERE run_id=? ORDER BY second`, runID)
	if err != nil {
		return nil, fmt.Errorf("mysql: list run series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []metrics.Interval
	for rows.Next() {
		var (
			iv  metrics.Interval
			raw []byte
		)
		if err := rows.Scan(&iv.Timestamp, &iv.Concurrency, &iv.Samples, &iv.Failed, &iv.Bytes, &raw); err != nil {
			return nil, fmt.Errorf("mysql: scan run series: %w", err)
		}
		if raw != nil {
			if err := decodeJSON(raw, &iv.Latency); err != nil {
				return nil, fmt.Errorf("mysql: decode series latency: %w", err)
			}
		}
		out = append(out, iv)
	}
	return out, rows.Err()
}

// seriesRow is one second's worth of new measurements, in the table's own
// shape: the two concurrency readings kept separate so a second's virtual
// users stay the max of the two, exactly as the working state keeps them.
type seriesRow struct {
	second     int64
	engine     int
	labels     int
	samples    int64
	failed     int64
	bytes      int64
	latency    metrics.Histogram
	hasBuckets bool
}

// mergeSeriesSeconds adds a batch's fresh intervals to the run's permanent
// series, inside Absorb's transaction: the sequence watermark has already
// filtered what this run has absorbed, so additive merges here cannot count a
// retry twice.
//
// The counts are additive in SQL; the latency histogram is a JSON column that
// can only be merged in Go, so every second's row is locked for the
// read-merge-write exactly as mergeLabels locks a label's row -- two shards
// flushing the same second at nearly the same moment is routine, and without
// the lock the second write would silently discard the first shard's buckets.
// The lock order is fixed (open, lock, write, each in second order), matching
// every other merge in Absorb, so concurrent shards cannot deadlock.
func mergeSeriesSeconds(ctx context.Context, tx *sql.Tx, runID int64, fresh []metrics.Interval) error {
	if len(fresh) == 0 {
		return nil
	}

	bySecond := make(map[int64][]metrics.Interval, len(fresh))
	for _, iv := range fresh {
		bySecond[iv.Timestamp] = append(bySecond[iv.Timestamp], iv)
	}
	seconds := make([]int64, 0, len(bySecond))
	for ts := range bySecond {
		seconds = append(seconds, ts)
	}
	sort.Slice(seconds, func(i, j int) bool { return seconds[i] < seconds[j] })

	rows := make([]seriesRow, 0, len(seconds))
	for _, ts := range seconds {
		merged, engine := report.MergeSecond(bySecond[ts])
		row := seriesRow{
			second: ts, engine: engine, labels: merged.Concurrency,
			samples: merged.Samples, failed: merged.Failed, bytes: merged.Bytes,
			latency: merged.Latency, hasBuckets: len(merged.Latency) > 0,
		}
		rows = append(rows, row)
	}

	// The insert makes every row exist so it can be locked; the select then
	// takes the lock on all of them, whether this call created them or
	// another shard's Absorb did.
	openArgs := make([]any, 0, len(rows)*2)
	openRows := make([]string, len(rows))
	inClause := make([]string, len(rows))
	inArgs := make([]any, 0, len(rows)+1)
	inArgs = append(inArgs, runID)
	for i, row := range rows {
		openRows[i] = "(?,?)"
		openArgs = append(openArgs, runID, row.second)
		inClause[i] = "?"
		inArgs = append(inArgs, row.second)
	}
	// #nosec G202 -- the concatenated spans are strings.Join over placeholder
	// lists whose every element is a literal "?" assigned above; every value
	// travels as a bound parameter. Same reasoning as mergeLabels.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO execution_report_series (run_id, second) VALUES `+strings.Join(openRows, ",")+`
		 ON DUPLICATE KEY UPDATE run_id=run_id`,
		openArgs...); err != nil {
		return fmt.Errorf("mysql: open series seconds: %w", err)
	}
	locked, err := tx.QueryContext(ctx,
		`SELECT second, latency FROM execution_report_series
		 WHERE run_id=? AND second IN (`+strings.Join(inClause, ",")+`) FOR UPDATE`,
		inArgs...)
	if err != nil {
		return fmt.Errorf("mysql: lock series seconds: %w", err)
	}
	stored := make(map[int64]metrics.Histogram, len(rows))
	for locked.Next() {
		var (
			second int64
			raw    []byte
		)
		if err := locked.Scan(&second, &raw); err != nil {
			_ = locked.Close()
			return fmt.Errorf("mysql: scan series latency: %w", err)
		}
		if raw == nil {
			continue
		}
		var h metrics.Histogram
		if err := decodeJSON(raw, &h); err != nil {
			_ = locked.Close()
			return fmt.Errorf("mysql: decode series latency: %w", err)
		}
		stored[second] = h
	}
	if err := locked.Err(); err != nil {
		_ = locked.Close()
		return err
	}
	_ = locked.Close()

	mergeRows := make([]string, len(rows))
	mergeArgs := make([]any, 0, len(rows)*8)
	for i, row := range rows {
		latency := any(nil)
		if row.hasBuckets || len(stored[row.second]) > 0 {
			merged := metrics.Histogram{}
			merged.Merge(stored[row.second])
			merged.Merge(row.latency)
			encoded, err := json.Marshal(merged)
			if err != nil {
				return fmt.Errorf("mysql: encode series latency: %w", err)
			}
			latency = encoded
		}
		mergeRows[i] = "(?,?,?,?,?,?,?,?)"
		mergeArgs = append(mergeArgs, runID, row.second, row.engine, row.labels,
			row.samples, row.failed, row.bytes, latency)
	}
	// #nosec G202 -- mergeRows elements are the literal "(?,?,?,?,?,?,?,?)";
	// values bound via mergeArgs.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO execution_report_series
		 (run_id, second, engine_concurrency, label_concurrency, samples, failed, bytes, latency)
		 VALUES `+strings.Join(mergeRows, ",")+`
		 ON DUPLICATE KEY UPDATE
			engine_concurrency=engine_concurrency+VALUES(engine_concurrency),
			label_concurrency=label_concurrency+VALUES(label_concurrency),
			samples=samples+VALUES(samples),
			failed=failed+VALUES(failed),
			bytes=bytes+VALUES(bytes),
			latency=VALUES(latency)`,
		mergeArgs...); err != nil {
		return fmt.Errorf("mysql: merge series seconds: %w", err)
	}
	return nil
}
