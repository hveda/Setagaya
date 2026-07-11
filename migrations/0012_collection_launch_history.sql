-- 0012_collection_launch_history: usage history per launch (v2-compatible final
-- shape of collection_launch_history2, including launch_id).
CREATE TABLE IF NOT EXISTS collection_launch_history2 (
    collection_id INT UNSIGNED NOT NULL,
    context       VARCHAR(20)  NOT NULL,
    owner         VARCHAR(50)  NOT NULL,
    engines_count INT UNSIGNED,
    nodes_count   INT UNSIGNED,
    vu            INT UNSIGNED,
    launch_id     INT UNSIGNED NOT NULL,
    started_time  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time      TIMESTAMP NULL DEFAULT NULL,
    KEY (collection_id, context, end_time),
    KEY (started_time, end_time),
    KEY (launch_id)
) CHARSET=utf8mb4;
