-- 0018_execution_report: what a run is judged on, one row per run.
--
-- Kept because it is the evidence a readiness judgement was made on, and must
-- outlive the engines, the metrics backend's retention, and the campaign. That
-- is why the percentiles are stored rather than recomputed: the per-shard
-- buckets they came from are gone once the pods are.
--
-- Requested and achieved load sit side by side: latency means little without
-- knowing whether the load that produced it was the load intended.
CREATE TABLE IF NOT EXISTS execution_report (
    run_id                    INT UNSIGNED NOT NULL PRIMARY KEY,
    execution_id              INT UNSIGNED NOT NULL,
    scenario_id               INT UNSIGNED NOT NULL DEFAULT 0,
    engine                    VARCHAR(32)  NOT NULL DEFAULT '',
    outcome                   VARCHAR(20)  NOT NULL,
    -- Sub-second precision so runs that start close together still order
    -- correctly for trend queries.
    started_at                DATETIME(6)  NOT NULL,
    ended_at                  DATETIME(6)  NOT NULL,
    requested_concurrency     INT UNSIGNED NOT NULL DEFAULT 0,
    requested_throughput      DOUBLE       NOT NULL DEFAULT 0,
    requested_duration_seconds INT UNSIGNED NOT NULL DEFAULT 0,
    achieved_concurrency      INT UNSIGNED NOT NULL DEFAULT 0,
    achieved_throughput       DOUBLE       NOT NULL DEFAULT 0,
    achieved_duration_seconds INT UNSIGNED NOT NULL DEFAULT 0,
    achieved_samples          BIGINT UNSIGNED NOT NULL DEFAULT 0,
    achieved_failed           BIGINT UNSIGNED NOT NULL DEFAULT 0,
    error_rate                DOUBLE       NOT NULL DEFAULT 0,
    -- Failures split by who caused them. Stored as three columns rather than a
    -- single total so no query can report a count of failures without saying
    -- which side it came from.
    attribution_target        BIGINT UNSIGNED NOT NULL DEFAULT 0,
    attribution_engine        BIGINT UNSIGNED NOT NULL DEFAULT 0,
    attribution_unknown       BIGINT UNSIGNED NOT NULL DEFAULT 0,
    latency                   JSON         NULL,
    labels                    JSON         NULL,
    KEY (execution_id, started_at),
    KEY (started_at)
) CHARSET=utf8mb4;
