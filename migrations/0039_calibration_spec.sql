-- 0039_calibration_spec: the numeric search bounds
-- (internal/domain/calibration.Spec's SeedQPS/MaxQPS/MaxSteps/HoldSeconds)
-- configured for a CalibrateEngine execution -- the only parts of Spec that
-- don't already have a home: the target-health criterion lives in
-- execution_criteria (0034, Phase 6's mechanism, reused rather than
-- duplicated), and the pinned pod size lives on the execution itself
-- (0035). One row per execution; a calibration is configured once, though
-- it may drive more than one job (calibration_job) over its life.
CREATE TABLE IF NOT EXISTS calibration_spec (
    execution_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    seed_qps     DOUBLE          NOT NULL,
    max_qps      DOUBLE          NOT NULL,
    max_steps    INT UNSIGNED    NOT NULL,
    hold_seconds INT UNSIGNED    NOT NULL
) CHARSET=utf8mb4;
