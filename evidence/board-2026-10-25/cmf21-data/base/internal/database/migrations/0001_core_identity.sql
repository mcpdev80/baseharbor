CREATE TABLE tenants (
    id uuid PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (slug <> ''),
    CHECK (name <> '')
);

CREATE TABLE external_identities (
    id uuid PRIMARY KEY,
    issuer text NOT NULL,
    subject text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject),
    CHECK (issuer <> ''),
    CHECK (subject <> '')
);

CREATE TABLE memberships (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    external_identity_id uuid NOT NULL REFERENCES external_identities(id) ON DELETE CASCADE,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, external_identity_id, role),
    CHECK (role <> '')
);

CREATE INDEX memberships_external_identity_idx
    ON memberships (external_identity_id);

CREATE INDEX memberships_tenant_idx
    ON memberships (tenant_id);
