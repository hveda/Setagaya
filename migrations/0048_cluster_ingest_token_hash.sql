-- 0048_cluster_ingest_token_hash: the at-rest form of a registered cluster's
-- ingest token (internal/domain/clusterregistry/token.go). A BYOC cluster's
-- engine fleet authenticates with a per-cluster bearer token so one
-- customer's pods never hold another customer's credential; the plane keeps
-- only SHA-256 of it (64 lowercase hex).
--
-- NULL for operator entries and any entry that has not minted a token; UNIQUE
-- because an ingest token must resolve to at most one cluster -- the ingest
-- path authenticates by looking the hash up.
ALTER TABLE cluster_registry
    ADD COLUMN ingest_token_hash CHAR(64) NULL,
    ADD UNIQUE KEY uk_cluster_ingest_token_hash (ingest_token_hash);
