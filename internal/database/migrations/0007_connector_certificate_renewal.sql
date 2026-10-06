ALTER TABLE connector_enrollment_grants
    ADD COLUMN previous_certificate_serial text;

-- One bounded overlap slot per existing node; no unbounded certificate history.
CREATE TABLE connector_certificate_overlap (
    tenant_id uuid NOT NULL,
    node_id text NOT NULL,
    certificate_serial text NOT NULL CHECK (certificate_serial ~ '^[0-9a-f]{1,128}$'),
    certificate_expires_at timestamptz NOT NULL,
    admit_until timestamptz NOT NULL,
    certificate_revoked boolean NOT NULL DEFAULT false,
    PRIMARY KEY (tenant_id, node_id),
    FOREIGN KEY (tenant_id, node_id) REFERENCES connector_nodes (tenant_id, node_id) ON DELETE CASCADE,
    CHECK (admit_until <= certificate_expires_at)
);
ALTER TABLE connector_certificate_overlap ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_certificate_overlap FORCE ROW LEVEL SECURITY;
CREATE POLICY connector_certificate_overlap_tenant_isolation ON connector_certificate_overlap
    USING (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid);
