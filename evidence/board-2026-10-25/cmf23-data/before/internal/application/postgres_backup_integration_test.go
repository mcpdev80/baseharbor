package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	testruntime "github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestPostgresBackupDisasterRecovery(t *testing.T) {
	if os.Getenv("BASEHARBOR_POSTGRES_BACKUP_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_POSTGRES_BACKUP_INTEGRATION=1 to run PostgreSQL backup integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	compose, err := testruntime.Resolve(ctx)
	if err != nil {
		t.Fatalf("DetectCompose() error = %v", err)
	}

	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	m := WithSQLInstances(New("backup-probe", "dev", true, false, false), "analytics", "primary")
	store := Store{Root: filepath.Join(t.TempDir(), ".baseharbor", "apps"), Namespace: "backup-probe-" + m.ApplicationID[:8]}
	if _, err := store.Create(m); err != nil {
		t.Fatalf("Store.Create() error = %v", err)
	}
	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatalf("EnsureRuntime() error = %v", err)
	}
	project := files.Project
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cleanupCancel()
		if err := compose.DestroyProject(cleanupCtx, project, files.Compose, files.Env); err != nil {
			t.Errorf("cleanup isolated SQL backup fixture: %v", err)
		}
	})
	if err := compose.ConfigProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("ConfigProject() error = %v", err)
	}
	if err := compose.UpProject(ctx, project, files.Compose, files.Env); err != nil {
		t.Fatalf("initial UpProject() error = %v", err)
	}
	waitForPostgresBackupRuntime(t, ctx, compose, m, files)

	values := map[string]string{
		"analytics": "analytics-before-disaster",
		"primary":   "primary-before-disaster",
	}
	for _, instance := range SQLInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		sql := "CREATE TABLE recovery_probe (value text NOT NULL); INSERT INTO recovery_probe VALUES ('" + values[instance] + "');"
		if _, err := compose.ExecProjectInput(ctx, project, files.Compose, files.Env, []byte(sql), service, "sh", "-ec", postgresBackupClientCommand(instance, "psql", "--no-psqlrc -v ON_ERROR_STOP=1")); err != nil {
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

	for _, instance := range SQLInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		query := "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='recovery_probe'"
		out, err := compose.ExecProject(ctx, project, files.Compose, files.Env, service, "sh", "-ec", postgresBackupClientCommand(instance, "psql", "--no-psqlrc -v ON_ERROR_STOP=1 -tAc "+shellQuote(query)))
		if err != nil || strings.TrimSpace(out) != "0" {
			t.Fatalf("instance %s fresh volume must prove an empty recovery table inventory: %v", instance, err)
		}
	}

	if err := RestorePostgresInstances(ctx, compose, m, files, backups); err != nil {
		t.Fatalf("RestorePostgresInstances() error = %v", err)
	}
	for _, instance := range SQLInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		out, err := compose.ExecProject(ctx, project, files.Compose, files.Env, service, "sh", "-ec", postgresBackupClientCommand(instance, "psql", "--no-psqlrc -v ON_ERROR_STOP=1 -tAc "+shellQuote("SELECT value FROM recovery_probe")))
		if err != nil {
			t.Fatalf("verify restored %s error = %v", instance, err)
		}
		if strings.TrimSpace(out) != values[instance] {
			t.Fatalf("restored %s value = %q, want %q", instance, strings.TrimSpace(out), values[instance])
		}
	}
}

func waitForPostgresBackupRuntime(t *testing.T, ctx context.Context, compose bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles) {
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
