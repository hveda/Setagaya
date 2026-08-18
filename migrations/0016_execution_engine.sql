-- 0016_execution_engine: the engine an execution runs on. Empty means the
-- deployment's configured default, so executions created before an operator
-- offered a choice keep working.
ALTER TABLE execution ADD COLUMN engine VARCHAR(32) NOT NULL DEFAULT '';
