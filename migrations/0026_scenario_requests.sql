-- 0026_scenario_requests: a portable scenario's declarative workload, as a
-- Taurus `scenarios:` YAML fragment. One row per scenario, mirroring
-- 0006_scenario_test_file's shape for the same "one blob-ish thing per
-- scenario" concept rather than widening the scenario table itself.
CREATE TABLE IF NOT EXISTS scenario_requests (
    scenario_id  INT UNSIGNED PRIMARY KEY,
    raw          MEDIUMTEXT   NOT NULL,
    updated_time DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) CHARSET=utf8mb4;
