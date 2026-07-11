-- 0001_project: baseline `project` table.
--
-- Schema-compatible with the v2 `project` table in its fully-evolved form
-- (setagaya/db: 20180823 create + 20181031 name width + 20210301 sid +
-- 2025091101 tenant_id/created_by/updated_by), so v2 and v3 can share one
-- database during migration. The tenant_id foreign key to rbac_tenants is
-- added in a later phase once that table exists.
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
