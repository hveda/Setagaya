-- 0004_execution_scenario: one scenario's load profile within an execution.
CREATE TABLE IF NOT EXISTS execution_scenario (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    execution_id INT UNSIGNED NOT NULL,
    scenario_id  INT UNSIGNED NOT NULL,
    concurrency  INT UNSIGNED NOT NULL,
    rampup       INT UNSIGNED NOT NULL,
    duration     INT UNSIGNED NOT NULL,
    engines      INT UNSIGNED,
    csv_split    TINYINT(1)   DEFAULT 0,
    created_time TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (execution_id, scenario_id)
) CHARSET=utf8mb4;
