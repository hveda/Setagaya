-- 0052_execution_tenant_backfill: an execution's tenant always equals its
-- project's tenant (spec Approach A) -- inherited here for every row that
-- predates Phase 20's executionapp/scenarioapp stamping it on create.
-- Depends on 0050 having already given every project a tenant.
UPDATE execution e
    JOIN project p ON p.id = e.project_id
    SET e.tenant_id = p.tenant_id
    WHERE e.tenant_id IS NULL;
