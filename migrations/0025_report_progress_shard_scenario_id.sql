-- 0025_report_progress_shard_scenario_id: key a shard's working state by the
-- scenario it belongs to, not shard index alone.
--
-- Ultrareview finding. shard_index is a StatefulSet ordinal, scoped to one
-- scenario's own pods -- it restarts at 0 for every scenario an execution
-- bundles into one run. Without scenario_id in the key, two scenarios' shard 0
-- collided on the same row: each push looked like the other scenario's pod
-- restarting, resetting the sequence watermark and silently discarding or
-- double-absorbing intervals, and corrupting the finished-shard count
-- finalisation depends on.
ALTER TABLE report_progress_shard
    ADD COLUMN scenario_id INT UNSIGNED NOT NULL DEFAULT 0 AFTER run_id,
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (run_id, scenario_id, shard_index);
