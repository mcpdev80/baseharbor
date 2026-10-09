package identityprovider

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestKeycloakSinglePostgresStartsWithoutPrivilegedEntrypoint(t *testing.T) {
	var compose struct {
		Services map[string]struct {
			User        string            `yaml:"user"`
			ReadOnly    bool              `yaml:"read_only"`
			CapDrop     []string          `yaml:"cap_drop"`
			SecurityOpt []string          `yaml:"security_opt"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []string          `yaml:"volumes"`
			Tmpfs       []string          `yaml:"tmpfs"`
			Command     []string          `yaml:"command"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte("services:\n"+keycloakSingleDataLayerCompose()), &compose); err != nil {
		t.Fatal(err)
	}
	db := compose.Services["keycloak-db"]
	if db.User != "postgres" || !db.ReadOnly || len(db.CapDrop) != 1 || db.CapDrop[0] != "ALL" || len(db.SecurityOpt) != 1 || db.SecurityOpt[0] != "no-new-privileges:true" {
		t.Fatalf("single Keycloak SQL lost non-root confinement: %+v", db)
	}
	if db.Environment["PGDATA"] != "/var/lib/postgresql/18/docker" || len(db.Tmpfs) != 2 {
		t.Fatal("non-root PostgreSQL data/scratch contract missing")
	}
	init := compose.Services["keycloak-db-tls-init"]
	if !strings.Contains(strings.Join(init.Volumes, "\n"), "keycloak-db-data:/target-data") || !strings.Contains(strings.Join(init.Command, "\n"), `chown "$$uid:$$gid" /target-data /target-data/18 /target-data/18/docker`) {
		t.Fatal("data volume ownership was not prepared before capability-free SQL start")
	}
}
