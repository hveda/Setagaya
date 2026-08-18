-- 0012_execution_launch_history: usage history per launch.
CREATE TABLE IF NOT EXISTS execution_launch_history (
    execution_id  INT UNSIGNED NOT NULL,
    context       VARCHAR(20)  NOT NULL,
    owner         VARCHAR(50)  NOT NULL,
    engines_count INT UNSIGNED,
    nodes_count   INT UNSIGNED,
    vu            INT UNSIGNED,
    launch_id     INT UNSIGNED NOT NULL,
    started_time  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time      TIMESTAMP NULL DEFAULT NULL,
    KEY (execution_id, context, end_time),
    KEY (started_time, end_time),
    KEY (launch_id)
) CHARSET=utf8mb4;
