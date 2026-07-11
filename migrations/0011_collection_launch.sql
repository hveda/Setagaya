-- 0011_collection_launch: active-launch guard, one open launch per collection
-- (v2-compatible).
CREATE TABLE IF NOT EXISTS collection_launch (
    id            INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    collection_id INT UNSIGNED NOT NULL UNIQUE
) CHARSET=utf8mb4;
