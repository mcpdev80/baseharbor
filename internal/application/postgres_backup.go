package application

import (
	"context"
	"errors"
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

func DumpPostgresInstances(ctx context.Context, runtime PostgresBackupRuntime, m Manifest, files RuntimeFiles) ([]PostgresBackup, error) {
	instances := PostgresInstanceNames(m)
	backups := make([]PostgresBackup, 0, len(instances))
	for _, instance := range instances {
		service := runtimeServiceName("postgres", instance)
		database := postgresDatabaseName(m, instance)
		out, err := runtime.ExecProject(
			ctx,
			RuntimeProjectName(m),
			files.Compose,
			files.Env,
			service,
			"pg_dump",
			"--clean",
			"--if-exists",
			"--no-owner",
			"--no-privileges",
			"--format=plain",
			"-U", "baseharbor",
			"-d", database,
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

func RestorePostgresInstances(ctx context.Context, runtime PostgresBackupRuntime, m Manifest, files RuntimeFiles, backups []PostgresBackup) error {
	if err := ValidatePostgresBackupSet(m, backups); err != nil {
		return err
	}
	byInstance := make(map[string][]byte, len(backups))
	for _, backup := range backups {
		byInstance[backup.Instance] = backup.SQL
	}
	for _, instance := range PostgresInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		database := postgresDatabaseName(m, instance)
		if _, err := runtime.ExecProjectInput(
			ctx,
			RuntimeProjectName(m),
			files.Compose,
			files.Env,
			byInstance[instance],
			service,
			"psql",
			"-v", "ON_ERROR_STOP=1",
			"-U", "baseharbor",
			"-d", database,
		); err != nil {
			return fmt.Errorf("restore postgres instance %s: %w", instance, err)
		}
	}
	if err := VerifyPostgresRuntime(ctx, runtimeVerifier{runtime: runtime}, m, files); err != nil {
		return fmt.Errorf("verify restored postgres runtime: %w", err)
	}
	return nil
}

func ValidatePostgresBackupSet(m Manifest, backups []PostgresBackup) error {
	expected := PostgresInstanceNames(m)
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

type runtimeVerifier struct {
	runtime PostgresBackupRuntime
}

func (v runtimeVerifier) ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error) {
	if v.runtime == nil {
		return "", errors.New("postgres backup runtime is nil")
	}
	return v.runtime.ExecProject(ctx, project, composeFile, envFile, service, args...)
}
