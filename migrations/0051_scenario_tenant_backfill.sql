-- 0051_scenario_tenant_backfill: a scenario's tenant always equals its
-- project's tenant (spec Approach A) -- inherited here for every row that
-- predates Phase 20's executionapp/scenarioapp stamping it on create.
-- Depends on 0050 having already given every project a tenant.
UPDATE scenario s
    JOIN project p ON p.id = s.project_id
    SET s.tenant_id = p.tenant_id
    WHERE s.tenant_id IS NULL;
