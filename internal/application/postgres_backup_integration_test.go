package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestPostgresBackupDisasterRecovery(t *testing.T) {
	if os.Getenv("BASEHARBOR_POSTGRES_BACKUP_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_POSTGRES_BACKUP_INTEGRATION=1 to run PostgreSQL backup integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("DetectCompose() error = %v", err)
	}

	store := Store{Root: filepath.Join(t.TempDir(), ".baseharbor", "apps")}
	m := WithPostgresInstances(New("backup-probe", "dev", true, false, false), "analytics", "primary")
	if _, err := store.Create(m); err != nil {
		t.Fatalf("Store.Create() error = %v", err)
	}
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatalf("EnsureRuntime() error = %v", err)
	}
	project := RuntimeProjectName(m)
	if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("ConfigProject() error = %v", err)
	}
	if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("initial UpProject() error = %v", err)
	}
	defer func() { _ = compose.DestroyProject(context.Background(), project, files.Compose, files.Env) }()
	waitForPostgresBackupRuntime(t, ctx, compose, m, files)

	values := map[string]string{
		"analytics": "analytics-before-disaster",
		"primary":   "primary-before-disaster",
	}
	for _, instance := range PostgresInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		database := postgresDatabaseName(m, instance)
		sql := "CREATE TABLE recovery_probe (value text NOT NULL); INSERT INTO recovery_probe VALUES ('" + values[instance] + "');"
		if _, err := compose.ExecProject(ctx, project, files.Compose, files.Env, service, "psql", "-v", "ON_ERROR_STOP=1", "-U", "baseharbor", "-d", database, "-c", sql); err != nil {
			t.Fatalf("seed %s error = %v", instance, err)
		}
	}

	backups, err := DumpPostgresInstances(ctx, compose, m, files)
	if err != nil {
		t.Fatalf("DumpPostgresInstances() error = %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("backup count = %d, want 2", len(backups))
	}

	if err := compose.DestroyProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("DestroyProject() error = %v", err)
	}
	if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("recreate UpProject() error = %v", err)
	}
	waitForPostgresBackupRuntime(t, ctx, compose, m, files)

	for _, instance := range PostgresInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		database := postgresDatabaseName(m, instance)
		if _, err := compose.ExecProject(ctx, project, files.Compose, files.Env, service, "psql", "-U", "baseharbor", "-d", database, "-tAc", "SELECT value FROM recovery_probe"); err == nil {
			t.Fatalf("instance %s still contained pre-disaster table after volume destruction", instance)
		}
	}

	if err := RestorePostgresInstances(ctx, compose, m, files, backups); err != nil {
		t.Fatalf("RestorePostgresInstances() error = %v", err)
	}
	for _, instance := range PostgresInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		database := postgresDatabaseName(m, instance)
		out, err := compose.ExecProject(ctx, project, files.Compose, files.Env, service, "psql", "-U", "baseharbor", "-d", database, "-tAc", "SELECT value FROM recovery_probe")
		if err != nil {
			t.Fatalf("verify restored %s error = %v", instance, err)
		}
		if strings.TrimSpace(out) != values[instance] {
			t.Fatalf("restored %s value = %q, want %q", instance, strings.TrimSpace(out), values[instance])
		}
	}
}

func waitForPostgresBackupRuntime(t *testing.T, ctx context.Context, compose bhruntime.Compose, m Manifest, files RuntimeFiles) {
	t.Helper()
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		if err := VerifyPostgresRuntime(ctx, compose, m, files); err == nil {
			return
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for PostgreSQL readiness: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("PostgreSQL runtime did not become ready: %v", lastErr)
}
