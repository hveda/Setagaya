-- 0011_execution_launch: active-launch guard, one open launch per execution.
CREATE TABLE IF NOT EXISTS execution_launch (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    execution_id INT UNSIGNED NOT NULL UNIQUE
) CHARSET=utf8mb4;
