-- 0013_tenant: multi-tenancy isolation boundary.
CREATE TABLE IF NOT EXISTS tenant (
    id           BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(50)  NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE',
    created_time TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) CHARSET=utf8mb4;
