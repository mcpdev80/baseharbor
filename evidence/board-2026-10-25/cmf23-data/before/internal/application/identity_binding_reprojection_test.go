package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityWorkloadBindingReprojectionReplacesReadOnlyFiles(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "identity-reprojection",
		Environment:   "dev",
		Services:      Services{Identity: true},
	}
	files := RuntimeFilesFor(Store{Root: t.TempDir()}, m)
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.ApplicationEnv, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	first := IdentityDiscovery{
		Issuer:                "https://identity.example.test",
		AuthorizationEndpoint: "https://identity.example.test/authorize",
		TokenEndpoint:         "https://identity.example.test/token",
		UserinfoEndpoint:      "https://identity.example.test/userinfo",
		JWKSURI:               "https://identity.example.test/jwks",
		EndSessionEndpoint:    "https://identity.example.test/logout",
	}
	if err := MaterializeIdentityBindingWithWorkloadDiscoveryMaterial(
		m, files, "keycloak", first, first, "client", "secret", []byte("TEST-CA"),
	); err != nil {
		t.Fatalf("initial projection: %v", err)
	}

	uriPath := filepath.Join(workloadServiceBindingProjectionDir(files), IdentityBindingName, "uri")
	info, err := os.Stat(uriPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o444 {
		t.Fatalf("initial uri mode = %o, want 444", got)
	}

	second := first
	second.Issuer = "https://identity-new.example.test"
	second.AuthorizationEndpoint = second.Issuer + "/authorize"
	second.TokenEndpoint = second.Issuer + "/token"
	second.UserinfoEndpoint = second.Issuer + "/userinfo"
	second.JWKSURI = second.Issuer + "/jwks"
	second.EndSessionEndpoint = second.Issuer + "/logout"

	if err := MaterializeIdentityBindingWithWorkloadDiscoveryMaterial(
		m, files, "keycloak", second, second, "client", "secret", []byte("TEST-CA-2"),
	); err != nil {
		t.Fatalf("reprojection: %v", err)
	}

	for _, name := range []string{"uri", "oidc.issuer", "ca.crt"} {
		path := filepath.Join(workloadServiceBindingProjectionDir(files), IdentityBindingName, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o444 {
			t.Fatalf("%s mode = %o, want 444", name, got)
		}
	}
	data, err := os.ReadFile(uriPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != second.Issuer {
		t.Fatalf("reprojected uri = %q, want %q", got, second.Issuer)
	}
}

func TestReadOnlyProjectionFileIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binding")
	if err := writeReadOnlyProjectionFile(path, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeReadOnlyProjectionFile(path, []byte("one\n")); err != nil {
		t.Fatalf("identical reprojection: %v", err)
	}
	if err := writeReadOnlyProjectionFile(path, []byte("two\n")); err != nil {
		t.Fatalf("changed reprojection: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "two\n" {
		t.Fatalf("content = %q", data)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o444 {
		t.Fatalf("mode = %o, want 444", info.Mode().Perm())
	}
}
