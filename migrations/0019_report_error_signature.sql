-- 0019_report_error_signature: a run's failure modes, one row per signature.
--
-- The key is (label, response_code, side) -- what Honryu named the request, what
-- the target answered, and who the failure belongs to. Engine wording is not
-- part of it: the same 404 reads as "Request to ... didn't succeed (404)",
-- "Not Found", or "Response code: 404" depending on the engine, so grouping by
-- message would split one failure into three and break a service's error
-- history the moment its execution moved to another engine.
--
-- Wordings are kept in exemplars, bounded by the domain, as evidence for a
-- human rather than as identity.
CREATE TABLE IF NOT EXISTS report_error_signature (
    run_id        INT UNSIGNED    NOT NULL,
    label         VARCHAR(255)    NOT NULL,
    response_code VARCHAR(16)     NOT NULL DEFAULT '',
    side          VARCHAR(16)     NOT NULL,
    count         BIGINT UNSIGNED NOT NULL DEFAULT 0,
    -- Derived from the run's sample count at write time so trend queries can
    -- compare runs of different sizes in SQL. The domain recomputes from count.
    share         DOUBLE          NOT NULL DEFAULT 0,
    exemplars     JSON            NULL,
    PRIMARY KEY (run_id, label, response_code, side),
    KEY (label, response_code, side)
) CHARSET=utf8mb4;
