-- 0042_report_cluster: the load origin recorded on a run's report
-- (internal/domain/report.Report.Cluster) -- the registered cluster the run
-- generated load from, sourced from the execution at finalize time.
--
-- Added after the baseline, like 0035/0041, since migrations are tracked by
-- filename. Defaults to '' (the deployment default cluster), so every report
-- written before multi-cluster existed reads back as the default, unchanged.
ALTER TABLE execution_report
    ADD COLUMN cluster VARCHAR(100) NOT NULL DEFAULT '' AFTER engine;
