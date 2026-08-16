-- 0007_execution_data: data files attached to an execution.
CREATE TABLE IF NOT EXISTS execution_data (
    filename     VARCHAR(191) NOT NULL,
    execution_id INT UNSIGNED NOT NULL,
    PRIMARY KEY (execution_id, filename)
) CHARSET=utf8mb4;
