-- 0002_scenario: a reusable workload definition (Taurus "scenarios").
CREATE TABLE IF NOT EXISTS scenario (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL,
    project_id   INT UNSIGNED NOT NULL,
    tenant_id    BIGINT       NULL,
    created_by   VARCHAR(255) NULL,
    updated_by   VARCHAR(255) NULL,
    created_time TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    KEY (project_id)
) CHARSET=utf8mb4;
