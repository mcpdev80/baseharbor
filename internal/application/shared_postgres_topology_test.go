package application

import (
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"go.yaml.in/yaml/v3"
)

func TestSharedPostgresDefaultAndExplicitHARealizations(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	for _, members := range []int{1, 3, 5} {
		m := New("demo", "dev", true, false, false)
		if members > 1 {
			m = WithAvailabilityOverride(WithHA(m, true), "sql", nil, members)
		}
		state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "dev", Applications: map[string]sharedBackendAppState{}}
		if err := selectSharedPostgresTopology(&state, m); err != nil {
			t.Fatal(err)
		}
		if state.PostgresMembers != members {
			t.Fatalf("member count = %d, want %d", state.PostgresMembers, members)
		}
		var rendered strings.Builder
		writeSharedPostgresCompose(&rendered, state)
		var model struct {
			Services map[string]struct {
				Image string `yaml:"image"`
				User  string `yaml:"user"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal([]byte("services:\n"+rendered.String()), &model); err != nil {
			t.Fatal(err)
		}
		postgres, etcd := 0, 0
		for _, service := range model.Services {
			if strings.Contains(service.Image, "spilo") {
				postgres++
			}
			if strings.Contains(service.Image, "etcd") {
				etcd++
			}
		}
		if members == 1 {
			if len(model.Services) != 1 || postgres != 0 || etcd != 0 {
				t.Fatalf("non-HA created hidden cluster/toolbox/proxy: %#v", model.Services)
			}
			server := model.Services[sharedPostgresService("dev")]
			if server.Image != "docker.io/library/postgres:18-alpine" || server.User != "70:70" {
				t.Fatalf("non-HA PostgreSQL must be native and non-root: %#v", server)
			}
			for _, required := range []string{"ssl=on", "hba_file=", "postgres-access", "shared-postgres-data-1:/var/lib/postgresql"} {
				if !strings.Contains(rendered.String(), required) {
					t.Fatalf("non-HA missing %s", required)
				}
			}
		} else if postgres != members || etcd != members || len(model.Services) != 2*members+2 {
			t.Fatalf("explicit HA realization changed: postgres=%d etcd=%d services=%d", postgres, etcd, len(model.Services))
		}
	}
}

func TestSharedPostgresTopologyConflictIsReadOnlyAndDoesNotDowngrade(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	for _, existing := range []int{1, 3} {
		root := t.TempDir()
		files := SharedBackendFilesAt(root, "local", "dev")
		if err := os.MkdirAll(files.Dir, 0700); err != nil {
			t.Fatal(err)
		}
		state := sharedBackendState{Version: sharedBackendStateVersion, Environment: "dev", PostgresMembers: existing, PostgresAdminCredential: "credentials/provider-admin"}
		if err := writeSharedBackendState(files.State, state); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(files.State)
		if err != nil {
			t.Fatal(err)
		}
		m := WithHA(New("demo", "dev", true, false, false), existing == 1)
		if err := CheckSharedPostgresTopologyAt(root, "local", m); err == nil || !strings.Contains(err.Error(), "topology conflict") {
			t.Fatalf("conflict not rejected: %v", err)
		}
		after, err := os.ReadFile(files.State)
		if err != nil || string(after) != string(before) {
			t.Fatal("preflight changed provider state")
		}
	}
}

func TestSharedPostgresDoesNotMigratePreFreezeState(t *testing.T) {
	if _, err := decodeSharedBackendState([]byte(`{"version":2,"environment":"dev"}`)); err == nil {
		t.Fatal("obsolete provider state accepted")
	}
	if _, err := decodeSharedBackendState([]byte(`{"version":3,"environment":"dev","postgres_admin_credential":"credentials/admin"}`)); err == nil {
		t.Fatal("missing topology accepted")
	}
}
