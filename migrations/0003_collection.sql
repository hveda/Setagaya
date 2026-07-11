-- 0003_collection: baseline `collection` table (evolved v2 schema).
CREATE TABLE IF NOT EXISTS collection (
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
