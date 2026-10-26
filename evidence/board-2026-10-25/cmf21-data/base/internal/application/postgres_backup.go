package application

import (
	"context"
	"fmt"
	"strings"
)

const MaxPostgresBackupBytes int64 = 512 << 20

type PostgresBackupRuntime interface {
	ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error)
	ExecProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, service string, args ...string) (string, error)
}

type PostgresBackup struct {
	Instance string
	SQL      []byte
}

func DumpPostgresInstancesAt(ctx context.Context, runtime PostgresBackupRuntime, m Manifest, files RuntimeFiles, dataDir, namespace string) ([]PostgresBackup, error) {
	if !UsesSharedPostgreSQL(m) {
		return DumpPostgresInstances(ctx, runtime, m, files)
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return nil, err
	}
	appKey := sharedBackendApplicationKey(m)
	app, ok := state.Applications[appKey]
	if !ok {
		return nil, fmt.Errorf("shared PostgreSQL application registration is missing")
	}
	if err := verifySharedPostgresStateOwnership(state, appKey); err != nil {
		return nil, fmt.Errorf("refuse shared PostgreSQL backup: %w", err)
	}
	backups := make([]PostgresBackup, 0, len(SQLInstanceNames(m)))
	for _, instance := range SQLInstanceNames(m) {
		resource, ok := app.SQL[instance]
		if !ok {
			return nil, fmt.Errorf("shared PostgreSQL resource %s is missing", instance)
		}
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return nil, fmt.Errorf("load shared PostgreSQL credential %s: %w", instance, err)
		}
		// Spilo owns its monitoring and administration objects in every
		// database. The application role cannot restore that provider state.
		// Preserve other schemas and extensions belonging to the application.
		command := fmt.Sprintf("IFS= read -r PGPASSWORD; export PGPASSWORD; exec pg_dump --clean --if-exists --no-owner --no-privileges --format=plain"+
			" --exclude-schema=metric_helpers --exclude-schema=user_management"+
			" --exclude-extension=pg_stat_statements --exclude-extension=pg_stat_kcache"+
			" --exclude-extension=set_user --exclude-extension=pg_mon"+
			" -h postgres-access -U %s -d %s", shellQuote(resource.Username), shellQuote(resource.Database))
		execution, service := shared, sharedPostgresService(state.Environment)
		if state.CoreSQL != nil {
			execution.Project, execution.Compose, execution.Env = state.CoreSQL.Project, state.CoreSQL.Compose, state.CoreSQL.Env
			service = "postgres-admin"
			command = strings.Replace(command, "export PGPASSWORD;", "export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem;", 1)
			command = strings.Replace(command, "-h postgres-access", "-h postgres", 1)
		}
		out, err := runtime.ExecProjectInput(
			ctx,
			execution.Project,
			execution.Compose,
			execution.Env,
			[]byte(password+"\n"),
			service,
			"sh", "-ec", command,
		)
		if err != nil {
			return nil, fmt.Errorf("dump shared postgres instance %s: %w", instance, err)
		}
		if int64(len(out)) > MaxPostgresBackupBytes {
			return nil, fmt.Errorf("dump postgres instance %s exceeds maximum backup size", instance)
		}
		backups = append(backups, PostgresBackup{Instance: instance, SQL: []byte(out)})
	}
	return backups, nil
}

func DumpPostgresInstances(ctx context.Context, runtime PostgresBackupRuntime, m Manifest, files RuntimeFiles) ([]PostgresBackup, error) {
	instances := SQLInstanceNames(m)
	backups := make([]PostgresBackup, 0, len(instances))
	for _, instance := range instances {
		service := runtimeServiceName("postgres", instance)
		out, err := runtime.ExecProject(
			ctx,
			files.Project,
			files.Compose,
			files.Env,
			service,
			"sh", "-ec", postgresBackupClientCommand(instance, "pg_dump", "--clean --if-exists --no-owner --no-privileges --format=plain"),
		)
		if err != nil {
			return nil, fmt.Errorf("dump postgres instance %s: %w", instance, err)
		}
		if int64(len(out)) > MaxPostgresBackupBytes {
			return nil, fmt.Errorf("dump postgres instance %s exceeds maximum backup size", instance)
		}
		backups = append(backups, PostgresBackup{Instance: instance, SQL: []byte(out)})
	}
	return backups, nil
}

func RestorePostgresInstancesAt(ctx context.Context, runtime PostgresBackupRuntime, m Manifest, files RuntimeFiles, dataDir, namespace string, backups []PostgresBackup) error {
	if !UsesSharedPostgreSQL(m) {
		return RestorePostgresInstances(ctx, runtime, m, files, backups)
	}
	if err := ValidatePostgresBackupSet(m, backups); err != nil {
		return err
	}
	shared := SharedBackendFilesAt(dataDir, namespace, m.Environment)
	state, err := loadSharedBackendState(shared.State, m.Environment)
	if err != nil {
		return err
	}
	appKey := sharedBackendApplicationKey(m)
	app, ok := state.Applications[appKey]
	if !ok {
		return fmt.Errorf("shared PostgreSQL application registration is missing")
	}
	if err := verifySharedPostgresStateOwnership(state, appKey); err != nil {
		return fmt.Errorf("refuse shared PostgreSQL restore: %w", err)
	}
	byInstance := make(map[string][]byte, len(backups))
	for _, backup := range backups {
		byInstance[backup.Instance] = backup.SQL
	}
	for _, instance := range SQLInstanceNames(m) {
		resource, ok := app.SQL[instance]
		if !ok {
			return fmt.Errorf("shared PostgreSQL resource %s is missing", instance)
		}
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared PostgreSQL credential %s for restore: %w", instance, err)
		}
		restoreInput := append([]byte(password+"\n"), byInstance[instance]...)
		command := "IFS= read -r PGPASSWORD; export PGPASSWORD; exec psql -h postgres-access -v ON_ERROR_STOP=1 -U " + shellQuote(resource.Username) + " -d " + shellQuote(resource.Database)
		execution, service := shared, sharedPostgresService(state.Environment)
		if state.CoreSQL != nil {
			execution.Project, execution.Compose, execution.Env = state.CoreSQL.Project, state.CoreSQL.Compose, state.CoreSQL.Env
			service = "postgres-admin"
			command = strings.Replace(command, "export PGPASSWORD;", "export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem;", 1)
			command = strings.Replace(command, "-h postgres-access", "-h postgres", 1)
		}
		if _, err := runtime.ExecProjectInput(
			ctx,
			execution.Project,
			execution.Compose,
			execution.Env,
			restoreInput,
			service,
			"sh", "-ec", command,
		); err != nil {
			return fmt.Errorf("restore shared postgres instance %s: %w", instance, err)
		}
		if err := verifySharedPostgresDatabaseOwnership(ctx, runtime, shared, m.Environment, resource); err != nil {
			return fmt.Errorf("verify restored shared postgres instance %s ownership: %w", instance, err)
		}
	}
	return nil
}

func RestorePostgresInstances(ctx context.Context, runtime PostgresBackupRuntime, m Manifest, files RuntimeFiles, backups []PostgresBackup) error {
	if err := ValidatePostgresBackupSet(m, backups); err != nil {
		return err
	}
	byInstance := make(map[string][]byte, len(backups))
	for _, backup := range backups {
		byInstance[backup.Instance] = backup.SQL
	}
	for _, instance := range SQLInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		if _, err := runtime.ExecProjectInput(
			ctx,
			files.Project,
			files.Compose,
			files.Env,
			byInstance[instance],
			service,
			"sh", "-ec", postgresBackupClientCommand(instance, "psql", "--no-psqlrc -v ON_ERROR_STOP=1"),
		); err != nil {
			return fmt.Errorf("restore postgres instance %s: %w", instance, err)
		}
		out, err := runtime.ExecProject(
			ctx,
			files.Project,
			files.Compose,
			files.Env,
			service,
			"sh", "-ec", postgresBackupClientCommand(instance, "psql", "--no-psqlrc -v ON_ERROR_STOP=1 -tAc 'SELECT 1'"),
		)
		if err != nil {
			return fmt.Errorf("verify restored postgres instance %s: %w", instance, err)
		}
		if strings.TrimSpace(out) != "1" {
			return fmt.Errorf("verify restored postgres instance %s: unexpected query result %q", instance, strings.TrimSpace(out))
		}
	}
	return nil
}

// Use the protected native container's retained credentials and TLS trust,
// matching the existing backend readiness owner. Credentials stay out of argv
// and restore SQL stays on stdin; provider roles are never guessed.
func postgresBackupClientCommand(instance, client, options string) string {
	return fmt.Sprintf(`PGPASSWORD="$POSTGRES_PASSWORD" PGHOST=%s PGPORT=5432 PGUSER="$POSTGRES_USER" PGDATABASE="$POSTGRES_DB" PGSSLMODE=verify-ca PGSSLROOTCERT=/run/baseharbor/tls/ca.pem %s %s`, shellQuote(postgresAccessService(instance)), client, options)
}

func ValidatePostgresBackupSet(m Manifest, backups []PostgresBackup) error {
	expected := SQLInstanceNames(m)
	if len(backups) != len(expected) {
		return fmt.Errorf("postgres backup instance count mismatch: got %d, want %d", len(backups), len(expected))
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, instance := range expected {
		expectedSet[instance] = struct{}{}
	}
	seen := make(map[string]struct{}, len(backups))
	for _, backup := range backups {
		if _, ok := expectedSet[backup.Instance]; !ok {
			return fmt.Errorf("postgres backup contains unexpected instance %q", backup.Instance)
		}
		if _, duplicate := seen[backup.Instance]; duplicate {
			return fmt.Errorf("postgres backup contains duplicate instance %q", backup.Instance)
		}
		seen[backup.Instance] = struct{}{}
		if int64(len(backup.SQL)) > MaxPostgresBackupBytes {
			return fmt.Errorf("postgres backup instance %s exceeds maximum backup size", backup.Instance)
		}
		if len(backup.SQL) == 0 || strings.TrimSpace(string(backup.SQL)) == "" {
			return fmt.Errorf("postgres backup instance %s is empty", backup.Instance)
		}
	}
	for _, instance := range expected {
		if _, ok := seen[instance]; !ok {
			return fmt.Errorf("postgres backup is missing instance %q", instance)
		}
	}
	return nil
}
