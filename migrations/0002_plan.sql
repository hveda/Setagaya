-- 0002_plan: baseline `plan` table (evolved v2 schema: name 100 + rbac columns).
CREATE TABLE IF NOT EXISTS plan (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL,
    project_id   INT UNSIGNED NOT NULL,
    tenant_id    BIGINT       NULL,
    created_by   VARCHAR(255) NULL,
    updated_by   VARCHAR(255) NULL,
    created_time TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    KEY (project_id)
) CHARSET=utf8mb4;
