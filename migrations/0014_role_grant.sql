-- 0014_role_grant: assignment of a code-defined role to a subject, optionally
-- scoped to a tenant. tenant_id 0 is the global (service-provider) scope, which
-- keeps the uniqueness guard NULL-free so re-granting is a clean no-op.
CREATE TABLE IF NOT EXISTS role_grant (
    id         BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    subject    VARCHAR(255) NOT NULL,
    email      VARCHAR(255) NOT NULL DEFAULT '',
    role_name  VARCHAR(64)  NOT NULL,
    tenant_id  BIGINT       NOT NULL DEFAULT 0,
    granted_by VARCHAR(255) NOT NULL DEFAULT '',
    granted_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_grant (subject, role_name, tenant_id),
    KEY idx_grant_subject (subject)
) CHARSET=utf8mb4;
