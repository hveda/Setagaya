-- 0009_execution_run_history: start/end record for every run.
CREATE TABLE IF NOT EXISTS execution_run_history (
    run_id       INT UNSIGNED NOT NULL UNIQUE,
    execution_id INT UNSIGNED NOT NULL,
    started_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time     TIMESTAMP NULL DEFAULT NULL,
    KEY (execution_id, started_time),
    KEY (execution_id, run_id)
) CHARSET=utf8mb4;
