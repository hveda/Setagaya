-- 0041_execution_cluster: which registered cluster an execution generates load
-- from (internal/domain/execution.Execution.Cluster / a clusterregistry name).
--
-- Added after the baseline, like 0035, since migrations are tracked by
-- filename. Defaults to '' (the deployment's default cluster -- the control
-- plane's own InClusterConfig), so every execution created before multi-cluster
-- existed keeps deploying exactly where it always did, the same "empty is the
-- default" convention engine/cpu/memory already use.
ALTER TABLE execution
    ADD COLUMN cluster VARCHAR(100) NOT NULL DEFAULT '';
