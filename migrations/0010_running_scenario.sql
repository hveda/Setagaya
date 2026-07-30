-- 0010_running_scenario: scenarios currently executing, scoped by deployment
-- context.
CREATE TABLE IF NOT EXISTS running_scenario (
    execution_id INT UNSIGNED NOT NULL,
    scenario_id  INT UNSIGNED NOT NULL,
    context      VARCHAR(20)  NOT NULL,
    started_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (execution_id, scenario_id),
    KEY (context)
) CHARSET=utf8mb4;
