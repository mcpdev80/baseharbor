DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'baseharbor_runtime') THEN
        CREATE ROLE baseharbor_runtime
            NOLOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOINHERIT
            NOREPLICATION
            NOBYPASSRLS;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'baseharbor_migrator') THEN
        CREATE ROLE baseharbor_migrator
            NOLOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOINHERIT
            NOREPLICATION
            NOBYPASSRLS;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO baseharbor_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO baseharbor_runtime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO baseharbor_runtime;

GRANT USAGE, CREATE ON SCHEMA public TO baseharbor_migrator;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO baseharbor_migrator;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO baseharbor_migrator;

ALTER DEFAULT PRIVILEGES FOR ROLE baseharbor_migrator IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO baseharbor_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE baseharbor_migrator IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO baseharbor_runtime;

-- Re-run this reconciliation after migrations for installations that predate
-- these roles. Fresh installations should provision roles before migrations so
-- the migrator owns new objects and default privileges apply automatically.
