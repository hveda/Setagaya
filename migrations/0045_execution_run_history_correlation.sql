-- 0045_execution_run_history_correlation: the trace id whose headers the run's
-- generated load actually carried -- the id Deploy minted and parked as
-- execution.pending_correlation_id (0044), stamped onto the run at StartRun.
--
-- This is the row that survives StopRun, so it is where a finalized report
-- reads the run's correlation id from: reading the execution's pending value
-- instead would show a later deploy's id once the execution is re-deployed.
-- Defaults to '' so every run started before Phase 10 reads as having no
-- correlation id, never an error.
ALTER TABLE execution_run_history
    ADD COLUMN correlation_id VARCHAR(32) NOT NULL DEFAULT '';
