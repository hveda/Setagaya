-- 0008_execution_run: the single active run per execution.
CREATE TABLE IF NOT EXISTS execution_run (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    execution_id INT UNSIGNED NOT NULL UNIQUE
) CHARSET=utf8mb4;
