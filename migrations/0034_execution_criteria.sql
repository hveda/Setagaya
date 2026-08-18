-- 0034_execution_criteria: an execution's configured Taurus pass/fail
-- expressions (e.g. "failures>10%", "p95>500ms"), one row per criterion,
-- in the order they were set. Mirrors 0006_scenario_test_file's own
-- "one blob-ish/list-ish thing per aggregate gets its own table" shape.
-- Never wired through to compile.Input.Criteria before this migration --
-- an execution's report.Outcome could already reflect a real bzt failure,
-- but nothing let a caller name specific pass/fail thresholds, which is
-- what campaign verdicts (Phase 6) need to name a failing service's
-- specific failing criteria rather than only its coarse outcome.
CREATE TABLE IF NOT EXISTS execution_criteria (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    execution_id BIGINT UNSIGNED NOT NULL,
    criterion    VARCHAR(255)    NOT NULL,
    INDEX idx_execution_criteria_execution (execution_id)
) CHARSET=utf8mb4;
