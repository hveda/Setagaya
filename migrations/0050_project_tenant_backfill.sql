-- 0050_project_tenant_backfill: adopt every tenant-less project into the
-- default tenant created by 0049. Must run before 0051/0052, which backfill
-- scenario/execution FROM project's tenant_id -- a project left NULL here
-- would propagate NULL to everything under it instead of adopting it.
UPDATE project
    SET tenant_id = (SELECT id FROM tenant WHERE name = 'default')
    WHERE tenant_id IS NULL;
