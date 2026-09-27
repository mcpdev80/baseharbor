package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSharedPostgresTwoApplicationIsolationBackupRestoreDestroy(t *testing.T) {
	if os.Getenv("BASEHARBOR_SHARED_POSTGRES_INTEGRATION") != "1" {
		t.Skip("set BASEHARBOR_SHARED_POSTGRES_INTEGRATION=1 to run shared PostgreSQL integration")
	}
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		t.Fatalf("DetectCompose() error = %v", err)
	}

	root := t.TempDir()
	dataDir := filepath.Join(root, "target-state")
	store := Store{Root: filepath.Join(root, "apps"), Namespace: "shared-pg-it"}
	issuer := serviceissuer.New(t)

	appA := WithSQLInstances(New("app-a", "dev", true, false, false), "default", "analytics")
	appB := WithSQLInstances(New("app-b", "dev", true, false, false), "default")
	for _, m := range []Manifest{appA, appB} {
		if _, err := store.Create(m); err != nil {
			t.Fatalf("Store.Create(%s) error = %v", m.Name, err)
		}
	}

	for _, m := range []Manifest{appA, appB} {
		if err := ReconcileReferenceProviderRegistryAt(dataDir, m); err != nil {
			t.Fatalf("ReconcileReferenceProviderRegistryAt(%s) error = %v", m.Name, err)
		}
	}

	filesA, err := EnsureRuntime(ctx, issuer, store, appA)
	if err != nil {
		t.Fatalf("EnsureRuntime(appA) error = %v", err)
	}
	filesB, err := EnsureRuntime(ctx, issuer, store, appB)
	if err != nil {
		t.Fatalf("EnsureRuntime(appB) error = %v", err)
	}
	if _, err := ReconcileSharedBackends(ctx, compose, issuer, dataDir, store.Namespace, appA, filesA); err != nil {
		t.Fatalf("ReconcileSharedBackends(appA) error = %v", err)
	}
	defer func() { _ = DestroyAllSharedBackendsAt(context.Background(), compose, dataDir, store.Namespace) }()
	if _, err := ReconcileSharedBackends(ctx, compose, issuer, dataDir, store.Namespace, appB, filesB); err != nil {
		t.Fatalf("ReconcileSharedBackends(appB) error = %v", err)
	}

	if err := VerifySharedPostgreSQL(ctx, compose, dataDir, store.Namespace, appA); err != nil {
		t.Fatalf("VerifySharedPostgreSQL(appA) error = %v", err)
	}
	if err := VerifySharedPostgreSQL(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("VerifySharedPostgreSQL(appB) error = %v", err)
	}

	shared := SharedBackendFilesAt(dataDir, store.Namespace, "dev")
	state, err := loadSharedBackendState(shared.State, "dev")
	if err != nil {
		t.Fatalf("loadSharedBackendState() error = %v", err)
	}
	aState := state.Applications[sharedBackendApplicationKey(appA)]
	bState := state.Applications[sharedBackendApplicationKey(appB)]

	adminSecret, err := readSharedBackendCredential(shared.Dir, state.PostgresAdminCredential)
	if err != nil {
		t.Fatalf("read provider admin credential: %v", err)
	}
	for label, files := range map[string]RuntimeFiles{"appA": filesA, "appB": filesB} {
		envData, err := os.ReadFile(files.Env)
		if err != nil {
			t.Fatalf("read %s runtime env: %v", label, err)
		}
		if strings.Contains(string(envData), adminSecret) || strings.Contains(string(envData), "baseharbor_admin") {
			t.Fatalf("%s application runtime env leaked provider admin identity", label)
		}
		err = filepath.WalkDir(files.Bindings, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(data), adminSecret) || strings.Contains(string(data), "baseharbor_admin") {
				return fmt.Errorf("provider admin identity leaked into binding %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s binding isolation: %v", label, err)
		}
	}

	for instance, value := range map[string]string{"default": "A-ONLY", "analytics": "A-ANALYTICS"} {
		seedSharedPostgresSentinel(t, ctx, compose, shared, aState.SQL[instance], value)
	}
	seedSharedPostgresSentinel(t, ctx, compose, shared, bState.SQL["default"], "B-ONLY")

	backupsA, err := DumpPostgresInstancesAt(ctx, compose, appA, filesA, dataDir, store.Namespace)
	if err != nil {
		t.Fatalf("DumpPostgresInstancesAt(appA) error = %v", err)
	}
	if len(backupsA) != 2 {
		t.Fatalf("appA backup count = %d, want 2", len(backupsA))
	}
	for _, backup := range backupsA {
		if strings.Contains(string(backup.SQL), "B-ONLY") {
			t.Fatalf("appA backup %s contains appB sentinel", backup.Instance)
		}
	}

	setSharedPostgresSentinel(t, ctx, compose, shared, aState.SQL["default"], "A-MUTATED")
	if err := RestorePostgresInstancesAt(ctx, compose, appA, filesA, dataDir, store.Namespace, backupsA); err != nil {
		t.Fatalf("RestorePostgresInstancesAt(appA) error = %v", err)
	}
	if got := readSharedPostgresSentinel(t, ctx, compose, shared, aState.SQL["default"]); got != "A-ONLY" {
		t.Fatalf("appA sentinel after restore = %q, want A-ONLY", got)
	}
	if got := readSharedPostgresSentinel(t, ctx, compose, shared, aState.SQL["analytics"]); got != "A-ANALYTICS" {
		t.Fatalf("appA analytics sentinel after restore = %q, want A-ANALYTICS", got)
	}
	if got := readSharedPostgresSentinel(t, ctx, compose, shared, bState.SQL["default"]); got != "B-ONLY" {
		t.Fatalf("appB sentinel changed during appA restore: %q", got)
	}

	if err := ReleaseSharedBackendApplication(ctx, compose, dataDir, store.Namespace, appA); err != nil {
		t.Fatalf("ReleaseSharedBackendApplication(appA) error = %v", err)
	}
	if err := ReleaseApplicationProviderRegistryAt(dataDir, appA); err != nil {
		t.Fatalf("ReleaseApplicationProviderRegistryAt(appA) error = %v", err)
	}
	state, err = loadSharedBackendState(shared.State, "dev")
	if err != nil {
		t.Fatalf("load state after appA destroy: %v", err)
	}
	if _, exists := state.Applications[sharedBackendApplicationKey(appA)]; exists {
		t.Fatalf("appA registration remains after destroy")
	}
	if _, exists := state.Applications[sharedBackendApplicationKey(appB)]; !exists {
		t.Fatalf("appB registration was removed by appA destroy")
	}
	if err := VerifySharedPostgreSQL(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("appB not usable after appA destroy: %v", err)
	}
	if got := readSharedPostgresSentinel(t, ctx, compose, shared, bState.SQL["default"]); got != "B-ONLY" {
		t.Fatalf("appB sentinel after appA destroy = %q, want B-ONLY", got)
	}
	if out, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService("dev"), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-tAc", "SELECT current_user"); err != nil {
		t.Fatalf("baseharbor_admin missing after appA destroy: %v", err)
	} else if strings.TrimSpace(out) != "baseharbor_admin" {
		t.Fatalf("provider admin after appA destroy = %q", strings.TrimSpace(out))
	}

	if err := ReleaseSharedBackendApplication(ctx, compose, dataDir, store.Namespace, appB); err != nil {
		t.Fatalf("ReleaseSharedBackendApplication(appB) error = %v", err)
	}
	if err := ReleaseApplicationProviderRegistryAt(dataDir, appB); err != nil {
		t.Fatalf("ReleaseApplicationProviderRegistryAt(appB) error = %v", err)
	}
	if _, err := os.Stat(shared.State); !os.IsNotExist(err) {
		t.Fatalf("shared provider state still exists after last consumer destroy: err=%v", err)
	}
}

func seedSharedPostgresSentinel(t *testing.T, ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, resource sharedPostgresResource, value string) {
	t.Helper()
	sql := "CREATE TABLE isolation_probe (value text NOT NULL); INSERT INTO isolation_probe VALUES (" + quotePostgresLiteral(value) + ");"
	execSharedPostgresAsApp(t, ctx, compose, shared, resource, sql)
}

func setSharedPostgresSentinel(t *testing.T, ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, resource sharedPostgresResource, value string) {
	t.Helper()
	sql := "UPDATE isolation_probe SET value=" + quotePostgresLiteral(value)
	execSharedPostgresAsApp(t, ctx, compose, shared, resource, sql)
}

func readSharedPostgresSentinel(t *testing.T, ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, resource sharedPostgresResource) string {
	t.Helper()
	password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
	if err != nil {
		t.Fatalf("read app credential: %v", err)
	}
	command := "IFS= read -r PGPASSWORD; export PGPASSWORD; exec psql -h 127.0.0.1 -U " + shellQuote(resource.Username) + " -d " + shellQuote(resource.Database) + " -tAc 'SELECT value FROM isolation_probe'"
	out, err := compose.ExecProjectInput(ctx, shared.Project, shared.Compose, shared.Env, []byte(password+"\n"), sharedPostgresService("dev"), "sh", "-ec", command)
	if err != nil {
		t.Fatalf("read sentinel from %s: %v", resource.Database, err)
	}
	return strings.TrimSpace(out)
}

func execSharedPostgresAsApp(t *testing.T, ctx context.Context, compose bhruntime.Compose, shared SharedBackendFiles, resource sharedPostgresResource, sql string) {
	t.Helper()
	password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
	if err != nil {
		t.Fatalf("read app credential: %v", err)
	}
	command := "IFS= read -r PGPASSWORD; export PGPASSWORD; exec psql -h 127.0.0.1 -U " + shellQuote(resource.Username) + " -d " + shellQuote(resource.Database) + " -v ON_ERROR_STOP=1 -c " + shellQuote(sql)
	if _, err := compose.ExecProjectInput(ctx, shared.Project, shared.Compose, shared.Env, []byte(password+"\n"), sharedPostgresService("dev"), "sh", "-ec", command); err != nil {
		t.Fatalf("execute as %s on %s: %v", resource.Username, resource.Database, err)
	}
}
