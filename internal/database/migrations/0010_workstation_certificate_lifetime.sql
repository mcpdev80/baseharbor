-- Extend workstation device certificates to the five-year contract maximum.
-- Connector enrollment remains capped at 24 hours.
ALTER TABLE workstation_enrollment_grants
    DROP CONSTRAINT workstation_enrollment_grants_certificate_ttl_seconds_check,
    ADD CONSTRAINT workstation_enrollment_grants_certificate_ttl_seconds_check
        CHECK (certificate_ttl_seconds BETWEEN 1 AND 157680000);
