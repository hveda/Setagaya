-- 0036_calibration_job: the runtime state of one engine-capacity search
-- (internal/domain/calibration.Job) -- one row per CalibrateEngine
-- execution trigger, distinct from the search Spec (0039) which is
-- configured once and can drive more than one job over an execution's life,
-- the same way an execution can have more than one run.
--
-- claimed_at is the lease a calibration controller (cmd/calibrator, or
-- cmd/scheduler hosting the same loop) holds while it is deploying, running,
-- and tearing down the one pod a step needs -- a multi-minute operation, not
-- a single quick transaction, so it cannot use phase alone as the claim
-- marker the way schedule_occurrence's status does. NULL means unclaimed;
-- claiming sets it to the claim time, and RecordStep/MarkFailed clear it
-- again once the step (or the whole job) is settled. A claim older than the
-- caller's own lease duration is treated as abandoned and reclaimable --
-- see ClaimNextStep.
CREATE TABLE IF NOT EXISTS calibration_job (
    id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    execution_id          BIGINT UNSIGNED NOT NULL,
    phase                 VARCHAR(16)     NOT NULL,
    step_count            INT UNSIGNED    NOT NULL DEFAULT 0,
    bracket_lo_requested  DOUBLE          NOT NULL DEFAULT 0,
    bracket_lo_achieved   DOUBLE          NOT NULL DEFAULT 0,
    bracket_hi_requested  DOUBLE          NOT NULL DEFAULT 0,
    next_requested_qps    DOUBLE          NOT NULL DEFAULT 0,
    saturated_by          VARCHAR(16)     NULL,
    per_pod_qps           DOUBLE          NULL,
    failure_reason        VARCHAR(500)    NULL,
    claimed_at            DATETIME(6)     NULL,
    created_time          TIMESTAMP       DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_calibration_job_execution (execution_id),
    INDEX idx_calibration_job_claimable (phase, claimed_at)
) CHARSET=utf8mb4;
