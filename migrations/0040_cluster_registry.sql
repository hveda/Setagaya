-- 0040_cluster_registry: registered clusters Honryu can generate load from
-- (internal/domain/clusterregistry.Cluster). One central control plane
-- reaches out to N registered clusters; the name IS the ClusterRef.
--
-- Every entry references a home-cluster k8s Secret (secret_ref) the scheduler
-- reads to build its client -- consumption is uniform regardless of origin.
-- byoc_credential holds a BYOC cluster's self-contained kubeconfig encrypted
-- at rest (ciphertext only, never plaintext; the envelope encryption is
-- applied above this table). It is NULL for operator-managed entries, whose
-- Secret the operator owns out of band.
CREATE TABLE IF NOT EXISTS cluster_registry (
    name             VARCHAR(100)  NOT NULL,
    api_url          VARCHAR(512)  NOT NULL,
    ca_cert          TEXT          NOT NULL,
    ingest_url       VARCHAR(512)  NOT NULL,
    sidecar_image    VARCHAR(512)  NOT NULL,
    namespace        VARCHAR(63)   NOT NULL,
    secret_ref       VARCHAR(253)  NOT NULL,
    origin           VARCHAR(16)   NOT NULL,
    byoc_credential  BLOB          NULL,
    created_by       VARCHAR(255)  NOT NULL,
    created_time     DATETIME(6)   NOT NULL,
    PRIMARY KEY (name)
) CHARSET=utf8mb4;
