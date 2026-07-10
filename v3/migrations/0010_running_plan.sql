-- 0010_running_plan: plans currently executing, scoped by deployment context
-- (v2-compatible final shape: url column dropped, context + started_time added).
CREATE TABLE IF NOT EXISTS running_plan (
    collection_id INT UNSIGNED NOT NULL,
    plan_id       INT UNSIGNED NOT NULL,
    context       VARCHAR(20)  NOT NULL,
    started_time  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (collection_id, plan_id),
    KEY (context)
) CHARSET=utf8mb4;
