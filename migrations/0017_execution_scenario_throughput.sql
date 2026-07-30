-- 0017_execution_scenario_throughput: target request rate for an entry, shared
-- across its engines. Zero means unlimited, matching Taurus's default when the
-- key is absent.
ALTER TABLE execution_scenario ADD COLUMN throughput INT UNSIGNED NOT NULL DEFAULT 0;
