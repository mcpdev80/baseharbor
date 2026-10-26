CREATE TABLE application_ownerships (
    application_name text PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (application_name <> '')
);

CREATE INDEX application_ownerships_tenant_idx
    ON application_ownerships (tenant_id);

ALTER TABLE application_ownerships ENABLE ROW LEVEL SECURITY;
ALTER TABLE application_ownerships FORCE ROW LEVEL SECURITY;

CREATE POLICY application_ownerships_tenant_isolation
    ON application_ownerships
    USING (
        tenant_id = current_setting('baseharbor.tenant_id', true)::uuid
    )
    WITH CHECK (
        tenant_id = current_setting('baseharbor.tenant_id', true)::uuid
    );
