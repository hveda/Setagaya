-- 0007_collection_data: data files attached to a collection.
CREATE TABLE IF NOT EXISTS collection_data (
    filename      VARCHAR(191) NOT NULL,
    collection_id INT UNSIGNED NOT NULL,
    PRIMARY KEY (collection_id, filename)
) CHARSET=utf8mb4;
