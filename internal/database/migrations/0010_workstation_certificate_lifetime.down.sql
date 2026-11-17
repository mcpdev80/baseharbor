-- Existing grants over 24 hours must expire or be explicitly reissued before
-- rolling this migration back.
ALTER TABLE workstation_enrollment_grants
    DROP CONSTRAINT workstation_enrollment_grants_certificate_ttl_seconds_check,
    ADD CONSTRAINT workstation_enrollment_grants_certificate_ttl_seconds_check
        CHECK (certificate_ttl_seconds BETWEEN 1 AND 86400);
