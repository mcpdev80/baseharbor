package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type sharedCoreTestIssuer struct {
	*serviceissuer.Issuer
	core bhruntime.Files
}

func (i sharedCoreTestIssuer) CoreRuntimeFiles() (bhruntime.Files, error) { return i.core, nil }

func newSharedCoreTestIssuer(t *testing.T, root, namespace string, ha bool) sharedCoreTestIssuer {
	t.Helper()
	issuer := serviceissuer.New(t)
	core, err := bhruntime.EnsureFilesForProjectAndResources(filepath.Join(root, "core-runtime"), bhruntime.SharedProjectName(namespace), bhruntime.SharedResourceProjectName(namespace), bhruntime.Ports{Postgres: 5432, OpenBao: 8200}, ha)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := issuer.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := bhruntime.CorePostgresCA(core)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bundle.PEM, 0644); err != nil {
		t.Fatal(err)
	}
	return sharedCoreTestIssuer{Issuer: issuer, core: core}
}

type coreSQLBindingRuntime struct {
	bhruntime.RuntimeProvider
	projects []string
	services []string
	started  int
}

func (r *coreSQLBindingRuntime) ConfigProject(context.Context, string, string, string) error {
	r.started++
	return nil
}
func (r *coreSQLBindingRuntime) UpProject(context.Context, string, string, string) error {
	r.started++
	return nil
}
func (r *coreSQLBindingRuntime) ExecProjectInput(_ context.Context, project, compose, env string, input []byte, service string, args ...string) (string, error) {
	r.projects = append(r.projects, project)
	r.services = append(r.services, service)
	return "", nil
}

func TestTwoSharedSQLApplicationsAcrossEnvironmentsReuseCore(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "shared")
	root := t.TempDir()
	issuer := newSharedCoreTestIssuer(t, root, "test", false)
	store := Store{Root: filepath.Join(root, "apps"), Namespace: "test"}
	runtime := &coreSQLBindingRuntime{}
	var resources []sharedPostgresResource
	for _, m := range []Manifest{New("first", "dev", true, false, false), New("second", "test", true, false, false)} {
		files, err := EnsureRuntime(context.Background(), issuer, store, m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReconcileSharedBackends(context.Background(), runtime, issuer, root, "test", m, files); err != nil {
			t.Fatal(err)
		}
		shared := SharedBackendFilesAt(root, "test", m.Environment)
		state, err := loadSharedBackendState(shared.State, m.Environment)
		if err != nil {
			t.Fatal(err)
		}
		if state.CoreSQL == nil || state.CoreSQL.Project != issuer.core.Project || state.PostgresAdminCredential != "" {
			t.Fatal("application created a separate shared PostgreSQL provider")
		}
		if err := verifySharedPostgresStateOwnership(state, sharedBackendApplicationKey(m)); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(shared.Compose)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "services: {}") || strings.Contains(string(data), "postgres-data") {
			t.Fatal("SQL-only shared consumers own physical SQL resources")
		}
		resources = append(resources, state.Applications[sharedBackendApplicationKey(m)].SQL[defaultServiceInstance])
	}
	if runtime.started != 0 || len(runtime.services) != 2 {
		t.Fatal("shared SQL consumer started a provider instead of creating database bindings")
	}
	for i, service := range runtime.services {
		if service != "postgres-admin" || runtime.projects[i] != issuer.core.Project {
			t.Fatal("shared SQL used another physical provider")
		}
	}
	if resources[0].Database == resources[1].Database || resources[0].Username == resources[1].Username || resources[0].CredentialReference == resources[1].CredentialReference {
		t.Fatal("shared consumers lost isolated databases, users or credentials")
	}
}
