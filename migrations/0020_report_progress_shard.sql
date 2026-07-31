-- 0020_report_progress_shard: each pod's position in its own measurement stream.
--
-- The high-water mark is what makes a duplicate recognisable without keeping a
-- row per measurement. A sidecar clears its pending intervals only once a push
-- succeeds, so a push whose response was lost is followed by a *superset* batch
-- carrying those intervals again beside new ones -- neither the batch nor the
-- timestamp separates them, but the per-interval sequence does.
--
-- stream_id names the sidecar instance. Sequences count from one per instance,
-- so a restarted pod starts again at one; without noticing the stream changed,
-- the watermark would discard everything it measures from then on.
CREATE TABLE IF NOT EXISTS report_progress_shard (
    run_id      INT UNSIGNED    NOT NULL,
    shard_index INT             NOT NULL,
    stream_id   VARCHAR(64)     NOT NULL DEFAULT '',
    seq         BIGINT UNSIGNED NOT NULL DEFAULT 0,
    finished    TINYINT(1)      NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, shard_index)
) CHARSET=utf8mb4;
