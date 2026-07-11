-- 0013_v3_tenant: multi-tenancy isolation boundary. v3-namespaced (additive)
-- because v3 roles are code-defined and this runs alongside the untouched v2
-- rbac_* tables during the strangler cutover.
CREATE TABLE IF NOT EXISTS v3_tenant (
    id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(50)  NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE',
    created_time TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) CHARSET=utf8mb4;
