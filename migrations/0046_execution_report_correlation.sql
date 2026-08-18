-- 0046_execution_report_correlation: the trace id whose headers the run's
-- generated load actually carried -- the id Deploy minted (0044), stamped onto
-- the run's history at StartRun (0045), and now kept on the report itself.
--
-- Reports are the evidence that outlives everything else, so the deep-link id
-- into a customer's own APM has to live here too: reading it back from the
-- run's history row would break the moment that history is pruned. Defaults to
-- '' so every report saved before Phase 10 reads as having no correlation id,
-- never an error.
ALTER TABLE execution_report
    ADD COLUMN correlation_id VARCHAR(32) NOT NULL DEFAULT '' AFTER cluster;
