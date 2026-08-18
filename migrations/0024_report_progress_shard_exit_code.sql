-- 0024_report_progress_shard_exit_code: bzt's exit code, once a shard has
-- finished on its own.
--
-- Added after the baseline rather than folded into 0020_report_progress_shard:
-- migrations are tracked by filename, and this is what a run's outcome is
-- derived from once every shard has reported (taurus.OutcomeFromExitCode).
-- Nullable: a shard whose pod was torn down before it could write one still
-- finishes, just without a code to report.
ALTER TABLE report_progress_shard
    ADD COLUMN exit_code INT NULL;
