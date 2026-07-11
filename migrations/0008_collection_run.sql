-- 0008_collection_run: the single active run per collection (v2-compatible).
CREATE TABLE IF NOT EXISTS collection_run (
    id            INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    collection_id INT UNSIGNED NOT NULL UNIQUE
) CHARSET=utf8mb4;
