-- 0037_calibration_job_step: the full, auditable step-by-step history a
-- job's domain decision-state (0036_calibration_job's own bracket/step_count
-- columns) deliberately does not carry -- see calibration.Job's own doc.
CREATE TABLE IF NOT EXISTS calibration_job_step (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    job_id         BIGINT UNSIGNED NOT NULL,
    seq            INT UNSIGNED    NOT NULL,
    requested_qps  DOUBLE          NOT NULL,
    achieved_qps   DOUBLE          NOT NULL,
    classification VARCHAR(32)     NOT NULL,
    created_time   TIMESTAMP       DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_calibration_job_step_job (job_id)
) CHARSET=utf8mb4;
