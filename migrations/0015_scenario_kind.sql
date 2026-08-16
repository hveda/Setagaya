-- 0015_scenario_kind: how a scenario's workload is expressed, and which engine
-- it is pinned to when it carries an engine-native artefact.
--
-- Added after the baseline rather than folded into 0002_scenario.sql: migrations
-- are tracked by filename, so editing an applied file would leave any database
-- that already recorded it without these columns.
ALTER TABLE scenario
    ADD COLUMN kind   VARCHAR(20) NOT NULL DEFAULT 'portable',
    ADD COLUMN engine VARCHAR(32) NOT NULL DEFAULT '';
