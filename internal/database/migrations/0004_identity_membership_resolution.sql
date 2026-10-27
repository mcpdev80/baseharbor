CREATE POLICY memberships_identity_resolution
    ON memberships
    FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM external_identities AS identity
            WHERE identity.id = memberships.external_identity_id
              AND identity.issuer = current_setting('baseharbor.identity_issuer', true)
              AND identity.subject = current_setting('baseharbor.identity_subject', true)
        )
    );
