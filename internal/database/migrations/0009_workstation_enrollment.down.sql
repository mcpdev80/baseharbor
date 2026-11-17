-- Rollback of workstation-enrollment production path (lane w96). Drops only the
-- workstation_* tables and their policies; leaves every connector_* table,
-- index, and policy untouched.
DROP TABLE IF EXISTS workstation_enrollment_registry;
DROP TABLE IF EXISTS workstation_enrollment_grants;
