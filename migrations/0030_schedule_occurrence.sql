-- 0030_schedule_occurrence: one computed fire time for a schedule and its
-- outcome, decided when it was reserved (creation, or a later horizon
-- extension) -- not deferred until cmd/scheduler actually fires it, so a
-- rejected occurrence is visible to the schedule's owner immediately.
CREATE TABLE IF NOT EXISTS schedule_occurrence (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    schedule_id    BIGINT UNSIGNED NOT NULL,
    fire_time      DATETIME(6)     NOT NULL,
    status         VARCHAR(16)     NOT NULL,
    reservation_id BIGINT UNSIGNED NULL,
    INDEX idx_schedule_occurrence_schedule (schedule_id)
) CHARSET=utf8mb4;
