-- 0021_report_progress_label: a run's measurements so far, per request.
--
-- The response-time buckets are merged here rather than percentiles being kept,
-- because percentiles cannot be combined afterwards -- the reason the whole
-- push path carries histograms.
CREATE TABLE IF NOT EXISTS report_progress_label (
    run_id  INT UNSIGNED    NOT NULL,
    label   VARCHAR(255)    NOT NULL,
    samples BIGINT UNSIGNED NOT NULL DEFAULT 0,
    failed  BIGINT UNSIGNED NOT NULL DEFAULT 0,
    latency JSON            NULL,
    PRIMARY KEY (run_id, label)
) CHARSET=utf8mb4;
