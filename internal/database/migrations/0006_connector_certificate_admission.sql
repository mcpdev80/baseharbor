ALTER TABLE connector_nodes
    ADD COLUMN certificate_revoked boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT connector_nodes_revoked_has_certificate
        CHECK (NOT certificate_revoked OR certificate_serial IS NOT NULL);
