CREATE TABLE connector_nodes (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    node_id text NOT NULL CHECK (node_id ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$'),
    target_id text NOT NULL CHECK (target_id ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$'),
    runtime text NOT NULL CHECK (runtime IN ('docker', 'podman')),
    certificate_serial text,
    certificate_expires_at timestamptz,
    PRIMARY KEY (tenant_id, node_id),
    CHECK ((certificate_serial IS NULL) = (certificate_expires_at IS NULL))
);
ALTER TABLE connector_nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_nodes FORCE ROW LEVEL SECURITY;
CREATE POLICY connector_nodes_tenant_isolation ON connector_nodes
    USING (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid);

CREATE TABLE connector_enrollment_grants (
    token_digest text PRIMARY KEY CHECK (token_digest ~ '^[0-9a-f]{64}$'),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    target_id text NOT NULL CHECK (target_id ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$'),
    node_id text NOT NULL CHECK (node_id ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$'),
    runtime text NOT NULL CHECK (runtime IN ('docker', 'podman')),
    nonce_digest text NOT NULL CHECK (nonce_digest ~ '^[0-9a-f]{64}$'),
    expires_at timestamptz NOT NULL,
    certificate_ttl_seconds bigint NOT NULL CHECK (certificate_ttl_seconds BETWEEN 1 AND 86400),
    consumed_at timestamptz,
    csr_digest text CHECK (csr_digest ~ '^[0-9a-f]{64}$'),
    FOREIGN KEY (tenant_id, node_id) REFERENCES connector_nodes (tenant_id, node_id) ON DELETE CASCADE,
    CHECK ((consumed_at IS NULL) = (csr_digest IS NULL))
);
CREATE INDEX connector_enrollment_grants_tenant_idx ON connector_enrollment_grants (tenant_id);
ALTER TABLE connector_enrollment_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_enrollment_grants FORCE ROW LEVEL SECURITY;
CREATE POLICY connector_enrollment_grants_tenant_isolation ON connector_enrollment_grants
    USING (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid);
