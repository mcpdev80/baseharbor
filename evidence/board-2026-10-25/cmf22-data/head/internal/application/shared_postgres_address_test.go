package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type sharedPostgresAddressRuntime struct{ coreSQLBindingRuntime }

func (r *sharedPostgresAddressRuntime) ExecProject(context.Context, string, string, string, string, ...string) (string, error) {
	return "accepting connections", nil
}

func TestSharedPostgresWorkloadAddressMatchesPhysicalOwner(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	for _, management := range []bool{false, true} {
		name := "standalone"
		if management {
			name = "management"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			var issuer serviceaccess.Issuer = serviceissuer.New(t)
			expected := sharedPostgresAlias()
			if management {
				issuer = newSharedCoreTestIssuer(t, root, "test", false)
				expected = "postgres"
			}
			store := Store{Root: filepath.Join(root, "apps"), Namespace: "test"}
			m := New("consumer", "dev", true, false, false)
			files, err := EnsureRuntime(t.Context(), issuer, store, m)
			if err != nil {
				t.Fatal(err)
			}
			runtime := &sharedPostgresAddressRuntime{}
			if _, err := ReconcileSharedBackends(t.Context(), runtime, issuer, root, store.Namespace, m, files); err != nil {
				t.Fatal(err)
			}
			host, err := os.ReadFile(filepath.Join(WorkloadServiceBindingProjectionDir(files), "postgres", "host"))
			if err != nil || strings.TrimSpace(string(host)) != expected {
				t.Fatalf("workload SQL address does not match selected physical owner: %q %v", host, err)
			}
			if !management {
				shared := SharedBackendFilesAt(root, store.Namespace, m.Environment)
				rendered, err := os.ReadFile(shared.Compose)
				if err != nil || !strings.Contains(string(rendered), "- "+expected) {
					t.Fatal("workload SQL address has no declared native alias")
				}
			}
		})
	}
}
