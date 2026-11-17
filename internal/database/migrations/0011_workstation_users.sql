-- Human owners of enrolled developer workstations. Keycloak remains the
-- authentication authority; this table contains only directory metadata and a
-- stable provider subject so device revocation can be grouped by person.
CREATE TABLE workstation_users (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    login text NOT NULL CHECK (login ~ '^[a-zA-Z0-9][a-zA-Z0-9._@_-]{0,127}$'),
    display_name text NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 200),
    email text NOT NULL CHECK (position('@' in email) > 1),
    keycloak_user_id text NOT NULL CHECK (length(trim(keycloak_user_id)) BETWEEN 1 AND 255),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'offboarded')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    offboarded_at timestamptz,
    PRIMARY KEY (tenant_id, login),
    UNIQUE (tenant_id, keycloak_user_id),
    UNIQUE (tenant_id, email)
);
ALTER TABLE workstation_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE workstation_users FORCE ROW LEVEL SECURITY;
CREATE POLICY workstation_users_tenant_isolation ON workstation_users
    USING (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid);

ALTER TABLE workstation_enrollment_grants ADD COLUMN owner_login text;
ALTER TABLE workstation_enrollment_grants
    ADD CONSTRAINT workstation_enrollment_grants_owner_fk
    FOREIGN KEY (tenant_id, owner_login) REFERENCES workstation_users(tenant_id, login);
ALTER TABLE workstation_enrollment_registry ADD COLUMN owner_login text;
ALTER TABLE workstation_enrollment_registry
    ADD CONSTRAINT workstation_enrollment_registry_owner_fk
    FOREIGN KEY (tenant_id, owner_login) REFERENCES workstation_users(tenant_id, login);
CREATE INDEX workstation_enrollment_registry_owner_idx
    ON workstation_enrollment_registry (tenant_id, owner_login);
