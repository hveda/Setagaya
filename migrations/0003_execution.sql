-- 0003_execution: the runnable unit grouping scenarios (Taurus "execution").
CREATE TABLE IF NOT EXISTS execution (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL,
    project_id   INT UNSIGNED NOT NULL,
    csv_split    TINYINT(1)   DEFAULT 0,
    tenant_id    BIGINT       NULL,
    created_by   VARCHAR(255) NULL,
    updated_by   VARCHAR(255) NULL,
    created_time TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    KEY (project_id)
) CHARSET=utf8mb4;
