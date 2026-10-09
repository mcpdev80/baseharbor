package identityprovider

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type coreSQLTestIssuer struct {
	*serviceissuer.Issuer
	core bhruntime.Files
}

func (i coreSQLTestIssuer) CoreRuntimeFiles() (bhruntime.Files, error) { return i.core, nil }

func newCoreSQLTestIssuer(t *testing.T, root string, ha bool) coreSQLTestIssuer {
	t.Helper()
	issuer := serviceissuer.New(t)
	core, err := bhruntime.EnsureFilesForProjectAndResources(filepath.Join(root, "core-runtime"), bhruntime.SharedProjectName("local"), bhruntime.SharedResourceProjectName("local"), bhruntime.Ports{Postgres: 5432, OpenBao: 8200}, ha)
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
	return coreSQLTestIssuer{Issuer: issuer, core: core}
}

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
	root := t.TempDir()
	_, err := EnsureCoreIdentity(context.Background(), runtime, newCoreSQLTestIssuer(t, root, false), root, "local", "64e3d34f-ff08-4f59-9696-215857eaaf84")
	if !errors.Is(err, stop) {
		t.Fatalf("unexpected bootstrap result: %v", err)
	}
}

func TestCoreIdentityExistingDiscoveryRetainsHTTPSPort(t *testing.T) {
	root := t.TempDir()
	spec := application.Manifest{Version: application.CurrentVersion, Name: "core", Environment: "prod", Services: application.Services{Identity: true}}
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "core"}
	files, err := ensureKeycloakFilesForPlacement(context.Background(), spec, newCoreSQLTestIssuer(t, root, false), root, "local", placement)
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
	beforeCompose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	beforeEnv, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := ExistingCoreRuntimeFiles(root, "local")
	if err != nil {
		t.Fatal(err)
	}
	if owned.Project != files.Project || owned.Compose != files.Compose || owned.ConsumerNetwork != files.ConsumerNetwork || owned.InternalNetwork != files.InternalNetwork {
		t.Fatal("existing Core Identity lost its owned runtime binding")
	}
	afterCompose, _ := os.ReadFile(files.Compose)
	afterEnv, _ := os.ReadFile(files.Env)
	if !bytes.Equal(beforeCompose, afterCompose) || !bytes.Equal(beforeEnv, afterEnv) {
		t.Fatal("existing runtime discovery changed provider material")
	}
	if err := os.Chmod(files.Compose, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingCoreRuntimeFiles(root, "local"); err == nil {
		t.Fatal("accepted unprotected Compose")
	}
	if err := os.Remove(files.Compose); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(files.Env, files.Compose); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingCoreRuntimeFiles(root, "local"); err == nil {
		t.Fatal("accepted symlink Compose")
	}
}
