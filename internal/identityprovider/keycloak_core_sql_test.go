package identityprovider

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"go.yaml.in/yaml/v3"
)

func TestSharedIdentityUsesExactlyOneCoreSQLDependency(t *testing.T) {
	for _, ha := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "ha"}[ha], func(t *testing.T) {
			root := t.TempDir()
			issuer := newCoreSQLTestIssuer(t, root, ha)
			placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "team-a"}
			app := application.WithHA(application.New("first", "dev", false, false, false), ha)
			first, err := ensureKeycloakFilesForPlacement(context.Background(), app, issuer, root, "local", placement)
			if err != nil {
				t.Fatal(err)
			}
			if err := setApplicationKeycloakCanonicalURL(first, "https://application-identity.localhost"); err != nil {
				t.Fatal(err)
			}
			canonical, err := readProtectedEnv(first.Env)
			if err != nil || canonical["BASEHARBOR_KEYCLOAK_CANONICAL_URL"] != first.PublicURL {
				t.Fatal("application reconciliation changed the installation identity authority")
			}
			app.Name, app.Environment = "second", "test"
			placement.SharingBoundary = "team-b"
			second, err := ensureKeycloakFilesForPlacement(context.Background(), app, issuer, root, "local", placement)
			if err != nil {
				t.Fatal(err)
			}
			if first.Project != second.Project || first.Dir != second.Dir || first.ConsumerNetwork != second.ConsumerNetwork || second.SharedSQL == nil || second.SharedSQL.Project != issuer.core.Project {
				t.Fatal("shared consumer provisioned a different physical dependency")
			}
			data, err := os.ReadFile(first.Compose)
			if err != nil {
				t.Fatal(err)
			}
			var graph struct {
				Services map[string]any `yaml:"services"`
				Volumes  map[string]any `yaml:"volumes"`
			}
			if err := yaml.Unmarshal(data, &graph); err != nil {
				t.Fatal(err)
			}
			want := 2
			if ha {
				want = 4
			}
			if len(graph.Services) != want || len(graph.Volumes) != 0 {
				t.Fatalf("shared Identity added SQL members/helpers/volumes: %d services, %d volumes", len(graph.Services), len(graph.Volumes))
			}
			for name := range graph.Services {
				if strings.HasPrefix(name, "keycloak-db") {
					t.Fatal("shared Identity starts its own PostgreSQL")
				}
			}
			for _, required := range []string{"jdbc:postgresql://postgres:5432/", "sslmode=verify-full", "sslrootcert=", "external: true", issuer.core.ResourceProject + "-default"} {
				if !bytes.Contains(data, []byte(required)) {
					t.Fatalf("missing shared SQL TLS binding %s", required)
				}
			}
			values, err := readProtectedEnv(first.Env)
			if err != nil {
				t.Fatal(err)
			}
			if values["BASEHARBOR_KEYCLOAK_DB_USER"] != "baseharbor_identity" || values["BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD"] != "" || values["BASEHARBOR_KEYCLOAK_DB_REPLICATION_PASSWORD"] != "" {
				t.Fatal("shared Identity owns Core SQL operator credentials")
			}
		})
	}
}

func TestSharedIdentityRetainedDedicatedSQLFailsBeforeMutation(t *testing.T) {
	for _, service := range []string{"keycloak-db", "keycloak-db-member-1"} {
		root := t.TempDir()
		issuer := newCoreSQLTestIssuer(t, root, false)
		dir := filepath.Join(root, "providers", "keycloak", "shared", "core")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		compose := []byte("services:\n  " + service + ":\n    image: retained\n")
		path := filepath.Join(dir, "compose.yaml")
		if err := os.WriteFile(path, compose, 0600); err != nil {
			t.Fatal(err)
		}
		app := application.New("core", "prod", false, false, false)
		placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "core"}
		_, err := ensureKeycloakFilesForPlacement(context.Background(), app, issuer, root, "local", placement)
		if err == nil || !strings.Contains(err.Error(), "migration") {
			t.Fatalf("retained SQL not protected: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(compose, after) {
			t.Fatal("retained Compose changed")
		}
		if _, err := os.Stat(filepath.Join(dir, "runtime.env")); !os.IsNotExist(err) {
			t.Fatal("migration refusal generated credentials")
		}
	}
}
