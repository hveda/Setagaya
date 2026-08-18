-- 0038_capacity_profile: what a calibration search
-- (internal/domain/capacityprofile.CapacityProfile) found -- one engine
-- pod's sustainable QPS for a (scenario, engine, pod size) combination.
--
-- The composite primary key IS the profile's identity (no calibration ever
-- needs a separate synthetic id): a later calibration for the same key
-- always supersedes the earlier one, via upsert.
CREATE TABLE IF NOT EXISTS capacity_profile (
    scenario_id          BIGINT UNSIGNED NOT NULL,
    engine               VARCHAR(32)     NOT NULL,
    cpu                  VARCHAR(32)     NOT NULL,
    memory               VARCHAR(32)     NOT NULL,
    per_pod_qps          DOUBLE          NOT NULL,
    saturated_by         VARCHAR(16)     NOT NULL,
    scenario_fingerprint VARCHAR(128)    NOT NULL,
    job_id               BIGINT UNSIGNED NOT NULL,
    calibrated_at        DATETIME(6)     NOT NULL,
    PRIMARY KEY (scenario_id, engine, cpu, memory)
) CHARSET=utf8mb4;
