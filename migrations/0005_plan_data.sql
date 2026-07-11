-- 0005_plan_data: non-JMX data files attached to a plan.
CREATE TABLE IF NOT EXISTS plan_data (
    filename VARCHAR(191) NOT NULL,
    plan_id  INT UNSIGNED NOT NULL,
    PRIMARY KEY (plan_id, filename)
) CHARSET=utf8mb4;
