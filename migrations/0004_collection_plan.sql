-- 0004_collection_plan: execution configuration joining a collection to plans.
CREATE TABLE IF NOT EXISTS collection_plan (
    id            INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    collection_id INT UNSIGNED NOT NULL,
    plan_id       INT UNSIGNED NOT NULL,
    concurrency   INT UNSIGNED NOT NULL,
    rampup        INT UNSIGNED NOT NULL,
    duration      INT UNSIGNED NOT NULL,
    engines       INT UNSIGNED,
    csv_split     TINYINT(1)   DEFAULT 0,
    created_time  TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (collection_id, plan_id)
) CHARSET=utf8mb4;
