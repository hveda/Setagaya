-- 0027_reservation: time-bounded engine-capacity claims against a tenant's
-- quota. Storage rounds to microsecond precision (MySQL DATETIME's limit) --
-- ample for real reservation windows, which are seconds to days, not the
-- nanosecond-scale boundary the domain's own Overlaps logic is tested at.
CREATE TABLE IF NOT EXISTS reservation (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_id    INT UNSIGNED    NOT NULL,
    cluster      VARCHAR(64)     NOT NULL DEFAULT '',
    engine_count INT UNSIGNED    NOT NULL,
    start_time   DATETIME(6)     NOT NULL,
    end_time     DATETIME(6)     NOT NULL,
    execution_id BIGINT UNSIGNED NOT NULL,
    -- Every window-overlap query is scoped to one tenant+cluster, so that is
    -- the index's leading edge; start/end let the overlap predicate
    -- (start_time < :end AND end_time > :start) use it directly.
    INDEX idx_reservation_window (tenant_id, cluster, start_time, end_time)
) CHARSET=utf8mb4;
