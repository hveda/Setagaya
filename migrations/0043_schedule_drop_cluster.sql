-- 0043_schedule_drop_cluster: a schedule no longer carries its own cluster.
--
-- The cluster a scheduled run deploys to -- and reserves quota against -- is
-- now the execution's (execution.Execution.Cluster), the single source of
-- truth (Phase 8). This also removed an inconsistency where the schedule stored
-- a cluster for quota but the fire path deployed to the control plane's own
-- cluster regardless. Existing schedules keep firing onto their execution's
-- cluster (the default for pre-Phase-8 executions).
ALTER TABLE schedule
    DROP COLUMN cluster;
