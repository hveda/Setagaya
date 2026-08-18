-- 0033_campaign_service: one participating service (project) and its
-- designated readiness execution for a campaign -- the only execution under
-- that project freeze exempts, and the one whose report decides that
-- service's verdict.
CREATE TABLE IF NOT EXISTS campaign_service (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    campaign_id  BIGINT UNSIGNED NOT NULL,
    project_id   BIGINT UNSIGNED NOT NULL,
    execution_id BIGINT UNSIGNED NOT NULL,
    INDEX idx_campaign_service_campaign (campaign_id),
    INDEX idx_campaign_service_project (project_id)
) CHARSET=utf8mb4;
