-- 0005_scenario_data: non-script data files attached to a scenario.
CREATE TABLE IF NOT EXISTS scenario_data (
    filename    VARCHAR(191) NOT NULL,
    scenario_id INT UNSIGNED NOT NULL,
    PRIMARY KEY (scenario_id, filename)
) CHARSET=utf8mb4;
