-- 0006_plan_test_file: the single JMX test file for a plan.
CREATE TABLE IF NOT EXISTS plan_test_file (
    filename VARCHAR(191) NOT NULL,
    plan_id  INT UNSIGNED PRIMARY KEY
) CHARSET=utf8mb4;
