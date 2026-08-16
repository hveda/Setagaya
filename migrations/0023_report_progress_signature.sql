-- 0023_report_progress_signature: failure modes seen so far, keyed as the
-- finished report keys them -- request label, response code, and the side the
-- failure belongs to. Engine wording is evidence in exemplars, never identity.
CREATE TABLE IF NOT EXISTS report_progress_signature (
    run_id        INT UNSIGNED    NOT NULL,
    label         VARCHAR(255)    NOT NULL,
    response_code VARCHAR(16)     NOT NULL DEFAULT '',
    side          VARCHAR(16)     NOT NULL,
    count         BIGINT UNSIGNED NOT NULL DEFAULT 0,
    exemplars     JSON            NULL,
    PRIMARY KEY (run_id, label, response_code, side)
) CHARSET=utf8mb4;
