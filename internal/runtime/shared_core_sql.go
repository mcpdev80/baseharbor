package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CoreDependencyProvider binds consumers to the already selected installation.
// It supplies no operator credentials and must never provision another Core.
type CoreDependencyProvider interface {
	CoreRuntimeFiles() (Files, error)
}

type SQLConsumerExecutor interface {
	ExecProjectInput(context.Context, string, string, string, []byte, string, ...string) (string, error)
}

var sqlConsumerName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
var sqlConsumerOwner = regexp.MustCompile(`^baseharbor:[a-z0-9:/_.-]+$`)

func CorePostgresCA(files Files) string {
	return filepath.Join(filepath.Dir(files.Compose), "providers", "postgresql", "runtime", "ca.pem")
}

// EnsureCoreSQLConsumer creates only a database and a restricted login inside
// the selected Core SQL provider. Existing unowned objects are never adopted.
// Secrets travel on stdin; diagnostics deliberately omit SQL and credentials.
func EnsureCoreSQLConsumer(ctx context.Context, executor SQLConsumerExecutor, files Files, database, user, password, owner string) error {
	if executor == nil || files.Project == "" || !sqlConsumerName.MatchString(database) || !sqlConsumerName.MatchString(user) || password == "" || !sqlConsumerOwner.MatchString(owner) || strings.ContainsAny(password, "\r\n\x00") {
		return errors.New("shared Core SQL consumer binding is incomplete or invalid")
	}
	for _, path := range []string{files.Compose, files.Env} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return errors.New("shared Core SQL requires protected existing provider files")
		}
	}
	credentials, err := LoadControlPlaneCredentials(files)
	if err != nil {
		return err
	}
	if strings.ContainsAny(credentials.PostgresInternalPassword, "\r\n\x00") {
		return errors.New("shared Core SQL operator credential is invalid")
	}
	sql := coreSQLConsumerStatement(database, user, password, owner)
	const script = "IFS= read -r PGPASSWORD || exit 1\nexport PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem PGCONNECT_TIMEOUT=5\nexec psql --no-psqlrc -h postgres -p 5432 -U \"$1\" -d postgres --set=ON_ERROR_STOP=1 --file=-"
	_, err = executor.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(credentials.PostgresInternalPassword+"\n"+sql), "postgres-admin", "sh", "-ec", script, "--", credentials.PostgresInternalUser)
	if err != nil {
		return errors.New("shared Core SQL consumer reconciliation failed; verify readiness and owned database/role before retrying")
	}
	return nil
}

func coreSQLConsumerStatement(database, user, password, owner string) string {
	literal := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	return fmt.Sprintf(`SET standard_conforming_strings = on;
SELECT pg_advisory_lock(hashtextextended(%[1]s, 0));
DO $ownership$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = %[2]s AND shobj_description(oid, 'pg_authid') IS DISTINCT FROM %[1]s) THEN
    RAISE EXCEPTION 'shared SQL role is not owned by this consumer';
  END IF;
  IF EXISTS (SELECT FROM pg_database WHERE datname = %[3]s AND shobj_description(oid, 'pg_database') IS DISTINCT FROM %[1]s) THEN
    RAISE EXCEPTION 'shared SQL database requires explicit verified migration';
  END IF;
END;
$ownership$;
BEGIN;
SELECT format('CREATE ROLE %%I LOGIN PASSWORD %%L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS', %[2]s, %[4]s) WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %[2]s)
\gexec
ALTER ROLE "%[5]s" WITH LOGIN PASSWORD %[4]s NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
COMMENT ON ROLE "%[5]s" IS %[1]s;
COMMIT;
SELECT format('CREATE DATABASE %%I OWNER %%I', %[3]s, %[2]s) WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = %[3]s)
\gexec
COMMENT ON DATABASE "%[6]s" IS %[1]s;
REVOKE ALL ON DATABASE postgres FROM PUBLIC;
REVOKE ALL ON DATABASE template1 FROM PUBLIC;
REVOKE ALL ON DATABASE "%[6]s" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE "%[6]s" TO "%[5]s";
\connect "%[6]s"
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA public TO "%[5]s";
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE "%[5]s" IN SCHEMA public REVOKE ALL ON TABLES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE "%[5]s" IN SCHEMA public REVOKE ALL ON SEQUENCES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE "%[5]s" IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM PUBLIC;
`, literal(owner), literal(user), literal(database), literal(password), user, database)
}
