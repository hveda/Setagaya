-- 0032_campaign: a PM-owned readiness event -- a window, and the services
-- participating in it (0033_campaign_service). Pure coordination layer; a
-- campaign holds no execution semantics of its own. aborted_at is NULL for
-- a campaign that has not been aborted; there is no separate status column
-- -- active/closed is derived from window_start/window_end/aborted_at
-- (campaign.Campaign.IsActive), the same way a run's phase is derived
-- rather than stored.
CREATE TABLE IF NOT EXISTS campaign (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(200)    NOT NULL,
    tenant_id    INT UNSIGNED    NOT NULL,
    window_start DATETIME(6)     NOT NULL,
    window_end   DATETIME(6)     NOT NULL,
    aborted_at   DATETIME(6)     NULL,
    INDEX idx_campaign_tenant (tenant_id),
    INDEX idx_campaign_window (window_start, window_end)
) CHARSET=utf8mb4;
