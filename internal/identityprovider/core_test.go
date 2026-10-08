package identityprovider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type coreHostnameRuntime struct {
	testKeycloakRuntime
	t    *testing.T
	stop error
}

func (r coreHostnameRuntime) ConfigProject(_ context.Context, _, _, env string) error {
	values, err := readProtectedEnv(env)
	if err != nil {
		r.t.Fatal(err)
	}
	want := "https://" + keycloakPublicHost + ":" + values["BASEHARBOR_KEYCLOAK_PUBLIC_PORT"]
	if values["BASEHARBOR_KEYCLOAK_CANONICAL_URL"] != want {
		r.t.Fatal("Core startup must bind Keycloak hostname to its owned HTTPS destination")
	}
	return r.stop
}

func TestCoreIdentityBindsHostnameBeforeRuntimeValidation(t *testing.T) {
	stop := errors.New("stop before runtime mutation")
	runtime := coreHostnameRuntime{t: t, stop: stop}
	_, err := EnsureCoreIdentity(context.Background(), runtime, serviceissuer.New(t), t.TempDir(), "local", "64e3d34f-ff08-4f59-9696-215857eaaf84")
	if !errors.Is(err, stop) {
		t.Fatalf("unexpected bootstrap result: %v", err)
	}
}

func TestCoreIdentityExistingDiscoveryRetainsHTTPSPort(t *testing.T) {
	root := t.TempDir()
	spec := application.Manifest{Version: application.CurrentVersion, Name: "core", Environment: "prod", Services: application.Services{Identity: true}}
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "core"}
	files, err := ensureKeycloakFilesForPlacement(context.Background(), spec, serviceissuer.New(t), root, "local", placement)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := existingCoreKeycloakFiles(files.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if existing.PublicPort != files.PublicPort || existing.AdminPort != files.AdminPort || existing.PublicURL != files.PublicURL || existing.AdminURL != files.AdminURL {
		t.Fatalf("readiness targets a different authority: existing=%+v provisioned=%+v", existing, files)
	}
	if files.Dir != filepath.Join(root, "providers", "keycloak", "shared", "core") {
		t.Fatalf("Core Identity lost installation placement: %s", files.Dir)
	}
	if files.Project == bhruntime.SharedProjectName("local") || files.Project != bhruntime.SharedProjectName("local-core") {
		t.Fatal("application provider reconciliation can replace Core Identity containers")
	}
}
