-- 0047_execution_orphan_completion: a shard's Final batch that arrived with no
-- open run to absorb it -- the signature of "engines already finished while
-- nobody triggered" (Phase 11's stranded-run guard) or of a pod that outlived
-- its run and flushed one last retry.
--
-- Trigger consults these rows to refuse opening a run for engines that are
-- already done (run.ErrEnginesFinished: re-deploy first); Deploy clears them,
-- because a new deploy is genuinely new engines. The exit code is kept when the
-- Final carried one -- it is real evidence about what those engines did, and a
-- reconciliation pass may still want it.
CREATE TABLE IF NOT EXISTS execution_orphan_completion (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    execution_id INT UNSIGNED    NOT NULL,
    scenario_id  INT UNSIGNED    NOT NULL,
    shard_index  INT UNSIGNED    NOT NULL,
    -- NULL when the Final batch arrived without an exit code: the shard said
    -- it was done but not how it ended.
    exit_code    INT             NULL,
    finished_at  DATETIME(6)     NOT NULL,
    -- Unique so a sidecar's retried Final is one event: the adapter upserts
    -- on this key rather than accumulating a row per retry.
    UNIQUE KEY (execution_id, scenario_id, shard_index),
    KEY (execution_id)
) CHARSET=utf8mb4;
