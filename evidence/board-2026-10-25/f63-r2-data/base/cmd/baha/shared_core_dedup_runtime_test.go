package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Runs inside both existing rootless provider gates, against their real Core.
// Counts native data services across projects, excluding access/admin helpers.
func verifySharedCoreDeduplicationFixture(t *testing.T, ctx context.Context, rt bhruntime.RuntimeProvider, namespace, dataDir string, core bhruntime.Files, identity identityprovider.KeycloakFiles, store application.Store, first application.Manifest, firstFiles application.RuntimeFiles) {
	t.Helper()
	issuer := platformopenbao.NewServiceIssuer(rt, core)
	second := application.WithHA(application.New("shared-dedup-second", "test", true, true, false), core.HA)
	secondFiles, err := application.EnsureRuntime(ctx, issuer, store, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(dataDir, second); err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := application.ReleaseSharedBackendApplication(cleanup, rt, dataDir, namespace, second); err != nil {
				t.Errorf("second consumer cleanup: %v", err)
			}
			if err := application.ReleaseApplicationProviderRegistryAt(dataDir, second); err != nil {
				t.Errorf("second registry cleanup: %v", err)
			}
		}
	}()
	for repeat := 0; repeat < 2; repeat++ {
		if _, err := application.ReconcileSharedBackends(ctx, rt, issuer, dataDir, namespace, second, secondFiles); err != nil {
			t.Fatal(err)
		}
	}
	for _, app := range []application.Manifest{first, second} {
		if err := application.VerifySharedBackends(ctx, rt, dataDir, namespace, app); err != nil {
			t.Fatalf("isolated shared consumer verification: %v", err)
		}
	}
	firstBinding, err := application.ResolveServiceBinding(firstFiles, "postgres", "default")
	if err != nil {
		t.Fatal(err)
	}
	secondBinding, err := application.ResolveServiceBinding(secondFiles, "postgres", "default")
	if err != nil {
		t.Fatal(err)
	}
	if firstBinding.Username == secondBinding.Username || firstBinding.Database == secondBinding.Database || firstBinding.Password == secondBinding.Password {
		t.Fatal("shared SQL consumers reused a database, role or credential")
	}
	query := func(binding application.ServiceBinding, sql string) string {
		const script = "IFS= read -r PGPASSWORD || exit 1; export PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem; exec psql --no-psqlrc -h postgres -U \"$1\" -d \"$2\" -At --set=ON_ERROR_STOP=1 --file=-"
		out, err := rt.ExecProjectInput(ctx, core.Project, core.Compose, core.Env, []byte(binding.Password+"\n"+sql+"\n"), "postgres-admin", "sh", "-ec", script, "--", binding.Username, binding.Database)
		if err != nil {
			t.Fatal("owned SQL operation failed")
		}
		return strings.TrimSpace(out)
	}
	query(secondBinding, "CREATE TABLE public.dedup_marker(value text); INSERT INTO public.dedup_marker VALUES ('second-retained');")
	backups, err := application.DumpPostgresInstancesAt(ctx, rt, second, secondFiles, dataDir, namespace)
	if err != nil {
		t.Fatal(err)
	}
	query(secondBinding, "DELETE FROM public.dedup_marker;")
	if err := application.RestorePostgresInstancesAt(ctx, rt, second, secondFiles, dataDir, namespace, backups); err != nil {
		t.Fatal(err)
	}
	if query(secondBinding, "SELECT value FROM public.dedup_marker;") != "second-retained" {
		t.Fatal("owned database backup/restore did not retain the marker")
	}
	shared := application.SharedBackendFilesAt(dataDir, namespace, first.Environment)
	assertInventory := func(label string, scopedProject string) {
		containers, err := rt.ListRuntimeContainers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sql, valkey, scoped := 0, 0, 0
		for _, c := range containers {
			if !c.Running {
				continue
			}
			if c.Project == core.Project || c.Project == identity.Project || c.Project == shared.Project || (scopedProject != "" && c.Project == scopedProject) {
				t.Logf("dedup inventory %s project=%s service=%s", label, c.Project, c.Service)
				if strings.HasPrefix(c.Service, "keycloak-db") {
					t.Fatal("shared Identity started a separate SQL deployment")
				}
				if c.Project == core.Project && strings.HasPrefix(c.Service, "postgres-member-") {
					sql++
				} else if c.Project == shared.Project && unexpectedSharedSQLDataService(c.Service) {
					t.Fatal("shared application started a separate SQL deployment")
				}
				if c.Project == shared.Project && strings.HasPrefix(c.Service, "shared-valkey-") && !strings.Contains(c.Service, "access") && !strings.Contains(c.Service, "sentinel") && !strings.Contains(c.Service, "ui") {
					valkey++
				}
				if scopedProject != "" && c.Project == scopedProject && c.Service == "postgres" {
					scoped++
				}
			}
		}
		members := 1
		if core.HA {
			members = 3
		}
		if sql != members || valkey != members || (scopedProject != "" && scoped != 1) {
			t.Fatalf("native data member counts SQL=%d Valkey=%d app-scoped=%d; shared expected=%d", sql, valkey, scoped, members)
		}
	}
	assertInventory("two-consumers", "")
	// Explicit application placement remains an independent deployment.
	previous := os.Getenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL))
	if err := os.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication)); err != nil {
		t.Fatal(err)
	}
	func() {
		defer os.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), previous)
		scoped := application.New("shared-dedup-independent", "dev", true, false, false)
		scopedFiles, err := application.EnsureRuntime(ctx, issuer, store, scoped)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := rt.DestroyProject(cleanup, scopedFiles.Project, scopedFiles.Compose, scopedFiles.Env); err != nil {
				t.Errorf("app-scoped owned cleanup: %v", err)
			}
		}()
		if err := rt.ConfigProject(ctx, scopedFiles.Project, scopedFiles.Compose, scopedFiles.Env); err != nil {
			t.Fatal(err)
		}
		if err := rt.UpProject(ctx, scopedFiles.Project, scopedFiles.Compose, scopedFiles.Env); err != nil {
			t.Fatal(err)
		}
		ready, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			err := application.VerifyPostgresRuntime(ready, rt, scoped, scopedFiles)
			if err == nil {
				break
			}
			select {
			case <-ready.Done():
				t.Fatal(err)
			case <-ticker.C:
			}
		}
		assertInventory("explicit-app-scoped-coexists", scopedFiles.Project)
	}()
	assertInventory("app-scoped-destroyed", "")
	if err := application.ReleaseSharedBackendApplication(ctx, rt, dataDir, namespace, second); err != nil {
		t.Fatal(err)
	}
	if err := application.ReleaseApplicationProviderRegistryAt(dataDir, second); err != nil {
		t.Fatal(err)
	}
	released = true
	if err := application.VerifySharedBackends(ctx, rt, dataDir, namespace, first); err != nil {
		t.Fatal(err)
	}
	if query(firstBinding, "SELECT value FROM public.provider_upgrade_marker;") != "application-retained" {
		t.Fatal("consumer destroy changed another application's data")
	}
	assertInventory("consumer-destroyed-core-retained", "")
}

func unexpectedSharedSQLDataService(service string) bool {
	// Core and Shared auxiliary services deliberately share the installation
	// project. The stable Core endpoint and admin toolbox are not data servers.
	if service == "postgres" || service == "postgres-admin" || strings.HasPrefix(service, "postgres-etcd-") || strings.HasSuffix(service, "-init") || strings.HasSuffix(service, "-access") || strings.HasSuffix(service, "-ui") {
		return false
	}
	return strings.Contains(service, "postgres")
}

func TestSharedCoreInventorySeparatesSQLHelpersFromData(t *testing.T) {
	for _, service := range []string{"postgres", "postgres-admin", "postgres-init", "postgres-etcd-1", "postgres-etcd-3", "shared-postgres-access", "shared-postgres-ui", "shared-valkey-core-shared-default"} {
		if unexpectedSharedSQLDataService(service) {
			t.Fatalf("helper/non-SQL service treated as another SQL deployment: %s", service)
		}
	}
	for _, service := range []string{"shared-postgres-dev", "shared-postgres-core-member-1", "keycloak-db-postgres"} {
		if !unexpectedSharedSQLDataService(service) {
			t.Fatalf("unexpected SQL data service accepted: %s", service)
		}
	}
}
