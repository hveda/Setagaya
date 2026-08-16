-- 0006_scenario_test_file: the single script/test file for a scenario.
CREATE TABLE IF NOT EXISTS scenario_test_file (
    filename    VARCHAR(191) NOT NULL,
    scenario_id INT UNSIGNED PRIMARY KEY
) CHARSET=utf8mb4;
