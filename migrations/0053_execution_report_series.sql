-- 0053_execution_report_series: one row per measured second of a run, merged
-- across pods and labels.
--
-- Intervals arrive per pod per label; the report keeps only their verdicts,
-- and the working state they accumulated through is discarded once the report
-- is saved (0018: "the per-shard buckets they came from are gone once the pods
-- are"). The shape of the run -- what a results page charts -- is evidence
-- too, so the merged second survives next to the report: written inside
-- Absorb's transaction, where a duplicate superset batch is filtered by the
-- shard's sequence watermark before it ever reaches this table, and kept
-- after finalisation rather than discarded with the working state.
--
-- Concurrency keeps both readings per second, as the working state does: the
-- engine's own aggregate row and the sum of the per-label rows, the larger of
-- the two being what a second's virtual users were.
CREATE TABLE IF NOT EXISTS execution_report_series (
    run_id             INT UNSIGNED NOT NULL,
    second             BIGINT       NOT NULL,
    engine_concurrency INT UNSIGNED NOT NULL DEFAULT 0,
    label_concurrency  INT UNSIGNED NOT NULL DEFAULT 0,
    samples            BIGINT UNSIGNED NOT NULL DEFAULT 0,
    failed             BIGINT UNSIGNED NOT NULL DEFAULT 0,
    bytes              BIGINT       NOT NULL DEFAULT 0,
    latency            JSON         NULL,
    PRIMARY KEY (run_id, second)
) CHARSET=utf8mb4;
