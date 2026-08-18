-- 0031_scheduler_horizon_run: a single overwritten row recording when
-- cmd/scheduler's recurring-schedule horizon-extension pass last completed
-- successfully. Queryable so a stalled extension job is observable rather
-- than silently leaving future occurrences unguarded (spec: horizon always
-- >= 7 days out while a schedule is active).
CREATE TABLE IF NOT EXISTS scheduler_horizon_run (
    id              TINYINT UNSIGNED NOT NULL PRIMARY KEY DEFAULT 1,
    last_success_at DATETIME(6)      NOT NULL,
    CONSTRAINT chk_scheduler_horizon_run_singleton CHECK (id = 1)
) CHARSET=utf8mb4;
