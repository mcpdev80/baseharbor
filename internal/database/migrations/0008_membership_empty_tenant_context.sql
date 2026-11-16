-- PostgreSQL resets transaction-local custom settings to an empty string on
-- a reused connection. An absent tenant must remain unscoped, not fail a UUID
-- cast before the separate verified-identity SELECT policy can be evaluated.
ALTER POLICY memberships_tenant_isolation ON memberships
    USING (
        tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(current_setting('baseharbor.tenant_id', true), '')::uuid
    );
