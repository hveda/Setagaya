-- 0029_schedule: a request to run an already-configured execution at a
-- future time, once (fire_at) or recurring (a cron recurrence). One row per
-- schedule; its computed fire times and their outcomes are the follow-on
-- 0030_schedule_occurrence table, not derived on the fly, so a rejected
-- occurrence stays visible even after its window has passed.
CREATE TABLE IF NOT EXISTS schedule (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    execution_id BIGINT UNSIGNED NOT NULL,
    tenant_id    INT UNSIGNED    NOT NULL,
    cluster      VARCHAR(64)     NOT NULL DEFAULT '',
    kind         VARCHAR(16)     NOT NULL,
    fire_at      DATETIME(6)     NULL,
    recurrence   VARCHAR(128)    NULL,
    active       TINYINT(1)      NOT NULL DEFAULT 1,
    INDEX idx_schedule_execution (execution_id)
) CHARSET=utf8mb4;
