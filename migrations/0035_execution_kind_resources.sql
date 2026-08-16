-- 0035_execution_kind_resources: what an execution is for (kind), and the
-- pod resources a calibration search pins its single step-pod to.
--
-- Added after the baseline rather than folded into 0003_execution.sql: see
-- 0015_scenario_kind's own note -- migrations are tracked by filename, so
-- editing an applied file would leave any database that already recorded it
-- without these columns.
--
-- kind defaults to 'normal' for every existing row (execution.KindNormal),
-- so an execution created before Kind existed keeps behaving exactly as it
-- always did. cpu/memory default to '' (unset, ports.DeploySpec's own
-- "optional resource request/limit" convention) -- only a CalibrateEngine
-- execution ever pins them; an ordinary execution's pods keep deploying at
-- the cluster's default size.
ALTER TABLE execution
    ADD COLUMN kind   VARCHAR(32) NOT NULL DEFAULT 'normal',
    ADD COLUMN cpu    VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN memory VARCHAR(32) NOT NULL DEFAULT '';
