-- 0028_tenant_quota: a tenant's engine quota ceiling, per cluster. Keyed by
-- (tenant_id, cluster) rather than a column on tenant, since a tenant can
-- have a different ceiling in each registered cluster (Phase 8). Absent
-- means unconfigured, read back as 0 by the application layer -- nothing
-- runs until a ceiling is explicitly set, never an accidental unlimited
-- default.
CREATE TABLE IF NOT EXISTS tenant_quota (
    tenant_id INT UNSIGNED NOT NULL,
    cluster   VARCHAR(64)  NOT NULL DEFAULT '',
    ceiling   INT UNSIGNED NOT NULL,
    PRIMARY KEY (tenant_id, cluster)
) CHARSET=utf8mb4;
