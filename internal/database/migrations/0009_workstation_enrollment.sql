-- Workstation-enrollment production path (lane w96): distinct namespace.
-- New tables only; no connector_* table, index, or policy is touched.
--
-- workstation_enrollment_grants holds the one-use bootstrap authorization.
-- A grant is one-use: consumed_at is set atomically on consume and stays set,
-- so a restarted process cannot replay a consumed bootstrap (fail-closed).
-- workstation_enrollment_registry persists the approved registry owner as a
-- FACT: the installation-scoped scope, the issuance time, and the separate
-- workstation SPIFFE identity issued by the enrollment.
--
-- token_digest / nonce_digest are the SHA-256 hex of the one-use bootstrap
-- credentials (see workstationenrollment.digest), matching the connector grant
-- convention, but the digest key is namespaced to this table.

CREATE TABLE workstation_enrollment_grants (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    station text NOT NULL CHECK (station ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$'),
    token_digest text NOT NULL CHECK (token_digest ~ '^[0-9a-f]{64}$'),
    nonce_digest text NOT NULL CHECK (nonce_digest ~ '^[0-9a-f]{64}$'),
    expires_at timestamptz NOT NULL,
    certificate_ttl_seconds bigint NOT NULL CHECK (certificate_ttl_seconds BETWEEN 1 AND 86400),
    consumed_at timestamptz,
    csr_digest text CHECK (csr_digest ~ '^[0-9a-f]{64}$'),
    PRIMARY KEY (tenant_id, station, token_digest),
    CHECK ((consumed_at IS NULL) = (csr_digest IS NULL))
);
ALTER TABLE workstation_enrollment_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE workstation_enrollment_grants FORCE ROW LEVEL SECURITY;
CREATE POLICY workstation_enrollment_grants_tenant_isolation ON workstation_enrollment_grants
    USING (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid);

CREATE TABLE workstation_enrollment_registry (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    station text NOT NULL CHECK (station ~ '^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$'),
    issued_at timestamptz NOT NULL,
    recipient text NOT NULL,
    scope text NOT NULL,
    serial text NOT NULL,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, station)
);
ALTER TABLE workstation_enrollment_registry ENABLE ROW LEVEL SECURITY;
ALTER TABLE workstation_enrollment_registry FORCE ROW LEVEL SECURITY;
CREATE POLICY workstation_enrollment_registry_tenant_isolation ON workstation_enrollment_registry
    USING (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid);
