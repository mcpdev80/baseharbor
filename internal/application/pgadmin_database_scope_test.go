package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func assertPgAdminApplicationDatabaseScope(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var servers struct {
		Servers map[string]struct{ MaintenanceDB, DBRestriction, Username string }
	}
	if err := json.Unmarshal(data, &servers); err != nil {
		t.Fatal(err)
	}
	if len(servers.Servers) == 0 {
		t.Fatal("no app database connection")
	}
	for _, server := range servers.Servers {
		if server.MaintenanceDB != "app_database" || server.DBRestriction != server.MaintenanceDB || server.Username != "app_user" {
			t.Fatal("pgAdmin exposes unrelated databases instead of the owned app database")
		}
	}
}

func TestPgAdminConnectionsShowOnlyTheirApplicationDatabase(t *testing.T) {
	t.Run("application", func(t *testing.T) {
		root := t.TempDir()
		ca := filepath.Join(root, "ca.pem")
		if err := os.WriteFile(ca, []byte("public-ca"), 0600); err != nil {
			t.Fatal(err)
		}
		files := RuntimeFiles{Dir: root}
		m := New("app", "dev", true, false, false)
		values := map[string]string{PostgresUIPasswordEnv: "ui-test-value", postgresRuntimeKey(defaultServiceInstance, "PASSWORD"): "sql-test-value", postgresRuntimeKey(defaultServiceInstance, "DB"): "app_database", postgresRuntimeKey(defaultServiceInstance, "USER"): "app_user", postgresTLSCAKey(defaultServiceInstance): ca}
		if err := ensurePostgresManagementUI(context.Background(), serviceissuer.New(t), files, m, values); err != nil {
			t.Fatal(err)
		}
		assertPgAdminApplicationDatabaseScope(t, filepath.Join(root, "providers/management-ui/postgres/servers.json"))
	})
	t.Run("shared", func(t *testing.T) {
		root := t.TempDir()
		core := bhruntime.Files{Compose: filepath.Join(root, "core/compose.yaml")}
		ca := bhruntime.CorePostgresCA(core)
		if err := os.MkdirAll(filepath.Dir(ca), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ca, []byte("public-ca"), 0600); err != nil {
			t.Fatal(err)
		}
		shared := SharedBackendFiles{Dir: root}
		ref := "credentials/app-password"
		if err := os.MkdirAll(filepath.Join(root, "credentials"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ref), []byte("sql-test-value"), 0600); err != nil {
			t.Fatal(err)
		}
		state := sharedBackendState{CoreSQL: &core, Environment: "dev", Applications: map[string]sharedBackendAppState{"app/dev": {Application: "app", Environment: "dev", SQL: map[string]sharedPostgresResource{"default": {Database: "app_database", Username: "app_user", CredentialReference: ref}}}}}
		if err := refreshSharedPostgresManagementUIConfig(shared, state); err != nil {
			t.Fatal(err)
		}
		assertPgAdminApplicationDatabaseScope(t, filepath.Join(root, "management-ui/postgres/servers.json"))
	})
}
