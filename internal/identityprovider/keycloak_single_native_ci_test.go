package identityprovider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// TestKeycloakSingleNativeSQLInCI verifies the smallest real Identity graph:
// one selected Keycloak member and its dedicated PostgreSQL dependency.
// Shared Core SQL is covered by the separate managed-Identity/Core journeys.
func TestKeycloakSingleNativeSQLInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_KEYCLOAK_SINGLE_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("real Keycloak/SQL belongs to selected Docker/Podman acceptance")
	}
	kind := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if kind != "docker" && kind != "podman" {
		t.Fatalf("select docker or podman for native Identity acceptance, got %q", kind)
	}
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(capability.ScopeApplication))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	provider, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	issuer := serviceissuer.New(t)
	root := t.TempDir()
	namespace := "keycloak-single-native-ci"
	app := application.WithIdentity(application.New("keycloak-single-native-ci", "test", false, false, false))
	store := application.Store{Root: filepath.Join(root, "application"), Namespace: namespace}
	appFiles := application.RuntimeFilesFor(store, app)
	if err := os.MkdirAll(appFiles.Bindings, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appFiles.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{appFiles.Env, appFiles.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	driver := NewKeycloakDriver(provider, app, appFiles, issuer, root, namespace)
	resource := capability.Resource{
		Application: app.Name, Kind: capability.Identity,
		Name: "default", Provider: capability.ProviderKeycloak,
	}
	binding := capability.Binding{
		Resource: resource,
		Identity: &capability.IdentityBinding{Scopes: []string{"openid", "profile", "email"}},
	}
	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	files, err := ExistingKeycloakFilesAt(app, root, namespace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 90*time.Second)
		defer stop()
		if err := provider.DestroyProject(cleanup, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("clean up native Keycloak/SQL fixture: %v", err)
		}
	})
	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	if files.SharedSQL != nil {
		t.Fatal("dedicated Identity fixture silently reused shared Core SQL")
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, container := range containers {
		if container.Project != files.Project {
			continue
		}
		service := strings.TrimSpace(container.Service)
		name := strings.TrimSpace(container.Name)
		for _, candidate := range []string{
			"keycloak-1", "keycloak-2", "keycloak-3",
			"keycloak-db", "keycloak-db-member-1", "keycloak-db-member-2",
			"keycloak-db-member-3", "keycloak-db-etcd-1",
			"keycloak-db-etcd-2", "keycloak-db-etcd-3",
		} {
			matches := service == candidate || service == "" && (
				name == candidate || strings.Contains(name, "-"+candidate+"-") ||
					strings.HasSuffix(name, "-"+candidate) ||
					strings.HasPrefix(name, candidate+"-"))
			if !matches {
				continue
			}
			// HA members may not even exist as stopped containers; count all.
			if candidate == "keycloak-1" || candidate == "keycloak-db" {
				if container.Running {
					count[candidate]++
				}
			} else {
				count[candidate]++
			}
		}
	}
	for _, required := range []string{"keycloak-1", "keycloak-db"} {
		if count[required] != 1 {
			t.Fatalf("%s requires one running %s, found %d; inventory=%+v", kind, required, count[required], containers)
		}
	}
	for _, forbidden := range []string{
		"keycloak-2", "keycloak-3", "keycloak-db-member-1",
		"keycloak-db-member-2", "keycloak-db-member-3",
		"keycloak-db-etcd-1", "keycloak-db-etcd-2", "keycloak-db-etcd-3",
	} {
		if count[forbidden] != 0 {
			t.Fatalf("%s default unexpectedly created %s (%d)", kind, forbidden, count[forbidden])
		}
	}
}
