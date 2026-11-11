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
			CapAdd      []string          `yaml:"cap_add"`
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
	if db.User != "" || !db.ReadOnly || len(db.CapDrop) != 1 || db.CapDrop[0] != "ALL" || strings.Join(db.CapAdd, ",") != "CHOWN,DAC_READ_SEARCH,FOWNER,SETGID,SETUID" || len(db.SecurityOpt) != 1 || db.SecurityOpt[0] != "no-new-privileges:true" {
		t.Fatalf("single Keycloak SQL must leave UID assignment to the runtime and remain confined: %+v", db)
	}
	if db.Environment["PGDATA"] != "/var/lib/postgresql/18/docker" || len(db.Tmpfs) != 2 {
		t.Fatal("non-root PostgreSQL data/scratch contract missing")
	}
	if _, ok := compose.Services["keycloak-db-tls-init"]; ok {
		t.Fatal("single Keycloak SQL must not use a root-only ownership init container")
	}
	if !strings.Contains(strings.Join(db.Volumes, "\n"), "server-key.pem:/run/baseharbor/db-tls/server-key.pem:ro") || !strings.Contains(strings.Join(db.Command, "\n"), "cp /run/baseharbor/db-tls/server-key.pem /tmp/server-key.pem") {
		t.Fatal("PostgreSQL must make a private tmpfs key copy as its runtime-assigned UID")
	}
}
