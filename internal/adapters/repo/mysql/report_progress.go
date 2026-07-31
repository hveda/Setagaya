package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/ports"
)

var _ ports.ReportProgress = (*Repository)(nil)

// Absorb merges a shard's new intervals into the run's working state.
//
// Everything happens in one transaction, and the shard's row is locked first.
// Several pods push at once, and two absorbing the same shard concurrently --
// the retry of a push whose response was lost, arriving beside the push that
// followed it -- would otherwise both read the same watermark and both count
// the overlap.
func (r *Repository) Absorb(ctx context.Context, b ports.ProgressBatch) error {
	if err := b.Validate(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	watermark, err := lockShard(ctx, tx, b)
	if err != nil {
		return err
	}

	// Accumulate only what is new, then merge that much into the run. Building a
	// small accumulator first means the domain decides what a measurement means
	// and this file only decides how to add it to what is stored.
	fresh := report.NewAccumulator()
	highest := watermark
	newIntervals := 0
	for _, iv := range b.Intervals {
		if iv.Seq <= watermark {
			continue // already absorbed; a retry re-sent it
		}
		fresh.Add(iv)
		newIntervals++
		if iv.Seq > highest {
			highest = iv.Seq
		}
	}

	if newIntervals > 0 {
		if err := mergeProgress(ctx, tx, b.RunID, fresh.Snapshot()); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE report_progress_shard SET seq=?, finished=finished|? WHERE run_id=? AND shard_index=?`,
		highest, boolToInt(b.Final), b.RunID, b.ShardIndex); err != nil {
		return fmt.Errorf("mysql: advance shard progress: %w", err)
	}
	return tx.Commit()
}

// lockShard takes the shard's row for update and returns the sequence already
// absorbed from the batch's stream.
func lockShard(ctx context.Context, tx *sql.Tx, b ports.ProgressBatch) (int64, error) {
	// The insert makes the row exist so it can be locked; the select then takes
	// the lock whether this call created it or another did.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO report_progress_shard (run_id, shard_index, stream_id, seq)
		 VALUES (?,?,?,0) ON DUPLICATE KEY UPDATE run_id=run_id`,
		b.RunID, b.ShardIndex, b.StreamID); err != nil {
		return 0, fmt.Errorf("mysql: open shard progress: %w", err)
	}

	var (
		streamID string
		seq      int64
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT stream_id, seq FROM report_progress_shard
		 WHERE run_id=? AND shard_index=? FOR UPDATE`,
		b.RunID, b.ShardIndex).Scan(&streamID, &seq); err != nil {
		return 0, fmt.Errorf("mysql: lock shard progress: %w", err)
	}

	// A different stream is a restarted pod, whose sequences begin again at one.
	// Keeping the old watermark would discard the rest of its run.
	if streamID != b.StreamID {
		if _, err := tx.ExecContext(ctx,
			`UPDATE report_progress_shard SET stream_id=?, seq=0 WHERE run_id=? AND shard_index=?`,
			b.StreamID, b.RunID, b.ShardIndex); err != nil {
			return 0, fmt.Errorf("mysql: reset shard stream: %w", err)
		}
		return 0, nil
	}
	return seq, nil
}

// mergeProgress adds one batch's worth of accumulation to what is stored.
func mergeProgress(ctx context.Context, tx *sql.Tx, runID int64, s report.Snapshot) error {
	// Concurrency is a plain sum, so the database can do it without reading.
	for _, sec := range s.Seconds {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO report_progress_second (run_id, second, engine_concurrency, label_concurrency)
			 VALUES (?,?,?,?)
			 ON DUPLICATE KEY UPDATE
				engine_concurrency=engine_concurrency+VALUES(engine_concurrency),
				label_concurrency=label_concurrency+VALUES(label_concurrency)`,
			runID, sec.Second, sec.Engine, sec.Labels); err != nil {
			return fmt.Errorf("mysql: merge second %d: %w", sec.Second, err)
		}
	}

	// Buckets and exemplars need the old value to merge against, so these are
	// read under the shard lock already held.
	for _, l := range s.Labels {
		if err := mergeLabel(ctx, tx, runID, l); err != nil {
			return err
		}
	}
	for _, sig := range s.Signatures {
		if err := mergeSignature(ctx, tx, runID, sig); err != nil {
			return err
		}
	}
	return nil
}

func mergeLabel(ctx context.Context, tx *sql.Tx, runID int64, l report.LabelProgress) error {
	var raw []byte
	err := tx.QueryRowContext(ctx,
		`SELECT latency FROM report_progress_label WHERE run_id=? AND label=?`,
		runID, l.Label).Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fmt.Errorf("mysql: read label progress: %w", err)
	default:
		var stored metrics.Histogram
		if err := decodeJSON(raw, &stored); err != nil {
			return fmt.Errorf("mysql: decode label latency: %w", err)
		}
		merged := metrics.Histogram{}
		merged.Merge(stored)
		merged.Merge(l.Latency)
		l.Latency = merged
	}

	latency, err := json.Marshal(l.Latency)
	if err != nil {
		return fmt.Errorf("mysql: encode label latency: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO report_progress_label (run_id, label, samples, failed, latency)
		 VALUES (?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
			samples=samples+VALUES(samples), failed=failed+VALUES(failed), latency=VALUES(latency)`,
		runID, l.Label, l.Samples, l.Failed, latency); err != nil {
		return fmt.Errorf("mysql: merge label %q: %w", l.Label, err)
	}
	return nil
}

func mergeSignature(ctx context.Context, tx *sql.Tx, runID int64, sig report.ErrorSignature) error {
	var raw []byte
	err := tx.QueryRowContext(ctx,
		`SELECT exemplars FROM report_progress_signature
		 WHERE run_id=? AND label=? AND response_code=? AND side=?`,
		runID, sig.Label, sig.ResponseCode, string(sig.Side)).Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fmt.Errorf("mysql: read signature progress: %w", err)
	default:
		var stored []string
		if err := decodeJSON(raw, &stored); err != nil {
			return fmt.Errorf("mysql: decode exemplars: %w", err)
		}
		// The domain owns how many wordings are kept and how long they may be;
		// re-merging through it keeps that bound in one place.
		sig.Exemplars = report.MergeExemplars(stored, sig.Exemplars)
	}

	exemplars, err := json.Marshal(sig.Exemplars)
	if err != nil {
		return fmt.Errorf("mysql: encode exemplars: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO report_progress_signature (run_id, label, response_code, side, count, exemplars)
		 VALUES (?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE count=count+VALUES(count), exemplars=VALUES(exemplars)`,
		runID, sig.Label, sig.ResponseCode, string(sig.Side), sig.Count, exemplars); err != nil {
		return fmt.Errorf("mysql: merge signature: %w", err)
	}
	return nil
}

// Snapshot returns what a run has accumulated, ready for report.Restore.
func (r *Repository) Snapshot(ctx context.Context, runID int64) (report.Snapshot, error) {
	var s report.Snapshot

	labels, err := r.db.QueryContext(ctx,
		`SELECT label, samples, failed, latency FROM report_progress_label
		 WHERE run_id=? ORDER BY label`, runID)
	if err != nil {
		return s, err
	}
	if s.Labels, err = scanLabelProgress(labels); err != nil {
		return report.Snapshot{}, err
	}

	seconds, err := r.db.QueryContext(ctx,
		`SELECT second, engine_concurrency, label_concurrency FROM report_progress_second
		 WHERE run_id=? ORDER BY second`, runID)
	if err != nil {
		return report.Snapshot{}, err
	}
	if s.Seconds, err = scanSecondProgress(seconds); err != nil {
		return report.Snapshot{}, err
	}

	sigs, err := r.db.QueryContext(ctx,
		`SELECT label, response_code, side, count, exemplars FROM report_progress_signature
		 WHERE run_id=? ORDER BY side, response_code, label`, runID)
	if err != nil {
		return report.Snapshot{}, err
	}
	if s.Signatures, err = scanSignatureProgress(sigs); err != nil {
		return report.Snapshot{}, err
	}
	return s, nil
}

func scanLabelProgress(rows *sql.Rows) ([]report.LabelProgress, error) {
	defer func() { _ = rows.Close() }()
	var out []report.LabelProgress
	for rows.Next() {
		var (
			l   report.LabelProgress
			raw []byte
		)
		if err := rows.Scan(&l.Label, &l.Samples, &l.Failed, &raw); err != nil {
			return nil, err
		}
		if err := decodeJSON(raw, &l.Latency); err != nil {
			return nil, fmt.Errorf("mysql: decode label latency: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func scanSecondProgress(rows *sql.Rows) ([]report.SecondProgress, error) {
	defer func() { _ = rows.Close() }()
	var out []report.SecondProgress
	for rows.Next() {
		var sec report.SecondProgress
		if err := rows.Scan(&sec.Second, &sec.Engine, &sec.Labels); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, rows.Err()
}

func scanSignatureProgress(rows *sql.Rows) ([]report.ErrorSignature, error) {
	defer func() { _ = rows.Close() }()
	var out []report.ErrorSignature
	for rows.Next() {
		var (
			sig  report.ErrorSignature
			side string
			raw  []byte
		)
		if err := rows.Scan(&sig.Label, &sig.ResponseCode, &side, &sig.Count, &raw); err != nil {
			return nil, err
		}
		sig.Side = report.Side(side)
		if err := decodeJSON(raw, &sig.Exemplars); err != nil {
			return nil, fmt.Errorf("mysql: decode exemplars: %w", err)
		}
		out = append(out, sig)
	}
	return out, rows.Err()
}

// ShardsFinished counts the shards that have sent their final batch.
func (r *Repository) ShardsFinished(ctx context.Context, runID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM report_progress_shard WHERE run_id=? AND finished=1`, runID).Scan(&n)
	return n, err
}

// Discard drops a run's working state once it has become a report.
func (r *Repository) Discard(ctx context.Context, runID int64) error {
	for _, table := range []string{
		"report_progress_label", "report_progress_second",
		"report_progress_signature", "report_progress_shard",
	} {
		if _, err := r.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE run_id=?", runID); err != nil {
			return fmt.Errorf("mysql: discard %s: %w", table, err)
		}
	}
	return nil
}
