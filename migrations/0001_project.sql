-- 0001_project: projects, the ownership root for scenarios and executions.
CREATE TABLE IF NOT EXISTS project (
    id           INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL,
    owner        VARCHAR(50)  NOT NULL,
    sid          VARCHAR(25)  NULL,
    tenant_id    BIGINT       NULL,
    created_by   VARCHAR(255) NULL,
    updated_by   VARCHAR(255) NULL,
    created_time TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    KEY (owner),
    KEY idx_project_tenant (tenant_id),
    KEY idx_project_created_by (created_by)
) CHARSET=utf8mb4;
