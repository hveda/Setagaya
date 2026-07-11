-- 0009_collection_run_history: start/end record for every run (v2-compatible).
CREATE TABLE IF NOT EXISTS collection_run_history (
    run_id        INT UNSIGNED NOT NULL UNIQUE,
    collection_id INT UNSIGNED NOT NULL,
    started_time  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time      TIMESTAMP NULL DEFAULT NULL,
    KEY (collection_id, started_time),
    KEY (collection_id, run_id)
) CHARSET=utf8mb4;
