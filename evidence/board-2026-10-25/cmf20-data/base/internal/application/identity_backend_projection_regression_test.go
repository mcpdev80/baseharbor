package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackendProjectionRetainsExistingIdentityMaterial(t *testing.T) {
	m := Manifest{Version: CurrentVersion, ApplicationID: MustNewApplicationID(), Name: "identity-backend-projection", Environment: "dev", Services: Services{Identity: true}}
	files := RuntimeFilesFor(Store{Root: t.TempDir()}, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	d := IdentityDiscovery{Issuer: "https://identity.example.test", AuthorizationEndpoint: "https://identity.example.test/authorize", TokenEndpoint: "https://identity.example.test/token", UserinfoEndpoint: "https://identity.example.test/userinfo", JWKSURI: "https://identity.example.test/jwks"}
	if err := MaterializeIdentityBindingWithWorkloadDiscoveryMaterial(m, files, "keycloak", d, d, "client", "test-only-secret", []byte("test-only-trust")); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, name := range []string{"ca.crt", "client-secret", "oidc.issuer"} {
		data, err := os.ReadFile(filepath.Join(workloadServiceBindingProjectionDir(files), IdentityBindingName, name))
		if err != nil {
			t.Fatal("initial identity receiver material unavailable")
		}
		before[name] = data
	}
	rootBefore, err := os.Stat(workloadServiceBindingProjectionDir(files))
	if err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ensureWorkloadServiceBindingProjection(m, files, values); err != nil {
		t.Fatal(err)
	}
	rootAfter, err := os.Stat(workloadServiceBindingProjectionDir(files))
	if err != nil || !os.SameFile(rootBefore, rootAfter) {
		t.Fatal("live-mounted binding root was replaced")
	}
	if err := VerifyIdentityBinding(m, files); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIdentityDiscoveryBinding(files, "keycloak", "client", d, d); err != nil {
		t.Fatal(err)
	}
	for name, expected := range before {
		data, err := os.ReadFile(filepath.Join(workloadServiceBindingProjectionDir(files), IdentityBindingName, name))
		if err != nil || string(data) != string(expected) {
			t.Fatalf("backend reprojection destroyed or changed existing identity receiver %s", name)
		}
	}
}
