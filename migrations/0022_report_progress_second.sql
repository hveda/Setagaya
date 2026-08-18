-- 0022_report_progress_second: concurrency per second of a run.
--
-- Kept per second, and summed across shards, because a shard only reports its
-- own virtual users: the run's concurrency is what every shard had in flight at
-- the same moment. Both readings are stored -- the engine's own aggregate row
-- and the sum of the per-label rows -- because which one is meaningful depends
-- on what the engine sent.
CREATE TABLE IF NOT EXISTS report_progress_second (
    run_id             INT UNSIGNED NOT NULL,
    second             BIGINT       NOT NULL,
    engine_concurrency BIGINT       NOT NULL DEFAULT 0,
    label_concurrency  BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, second)
) CHARSET=utf8mb4;
