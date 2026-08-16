-- 0044_execution_pending_correlation: the trace id a Deploy minted for the run
-- it precedes, held on the execution until the next Trigger stamps it onto the
-- run (execution_run_history.correlation_id, migration 0045).
--
-- It is pending state, not part of the Execution aggregate: a run id does not
-- exist at compile/deploy time (StartRun happens later, in Trigger), so the
-- freshly minted id parks here. Last deploy wins -- the next Trigger always
-- runs against the pods the latest Deploy created. Defaults to '' so every
-- execution created before Phase 10 reads as having no pending id, never an
-- error.
ALTER TABLE execution
    ADD COLUMN pending_correlation_id VARCHAR(32) NOT NULL DEFAULT '';
