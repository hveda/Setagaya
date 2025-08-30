USE setagaya;
ALTER TABLE running_plan ADD COLUMN context varchar(20) NOT NULL AFTER plan_id,
ADD INDEX (context);