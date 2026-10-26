ALTER POLICY memberships_tenant_isolation ON memberships
    USING (
        tenant_id = current_setting('baseharbor.tenant_id', true)::uuid
    )
    WITH CHECK (
        tenant_id = current_setting('baseharbor.tenant_id', true)::uuid
    );
