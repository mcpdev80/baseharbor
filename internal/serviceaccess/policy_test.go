package serviceaccess

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultsToManagedLocalTLS(t *testing.T) {
	t.Setenv(EnvPKISource, "")
	p, err := Resolve("dev", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if !p.TLSRequired || p.PKISource != PKIManagedLocal {
		t.Fatalf("unexpected policy: %+v", p)
	}
	if p.AuthenticationRequired {
		t.Fatalf("dev should keep authentication optional while TLS remains mandatory: %+v", p)
	}
}

func TestResolveManagedEnvironmentRequiresAuthentication(t *testing.T) {
	p, err := Resolve("prod", "loki", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if !p.AuthenticationRequired {
		t.Fatalf("prod must require authentication: %+v", p)
	}
}

func TestResolveExternalPKI(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"server.pem", "server-key.pem", "ca.pem", "client.pem", "client-key.pem"} {
		mode := os.FileMode(0o644)
		if name == "server-key.pem" || name == "client-key.pem" {
			mode = 0o600
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("placeholder"), mode); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(EnvPKISource, string(PKIExternal))
	t.Setenv(EnvTLSCertFile, filepath.Join(dir, "server.pem"))
	t.Setenv(EnvTLSKeyFile, filepath.Join(dir, "server-key.pem"))
	t.Setenv(EnvTrustBundle, filepath.Join(dir, "ca.pem"))
	t.Setenv(EnvClientCertFile, filepath.Join(dir, "client.pem"))
	t.Setenv(EnvClientKeyFile, filepath.Join(dir, "client-key.pem"))
	p, err := Resolve("prod", "tempo", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if p.PKISource != PKIExternal || p.ClientCertificate == "" {
		t.Fatalf("external PKI not resolved: %+v", p)
	}
}

func TestProviderSpecificOverrideWins(t *testing.T) {
	t.Setenv(EnvPKISource, string(PKIManagedLocal))
	t.Setenv("BASEHARBOR_PROMETHEUS_PKI_SOURCE", string(PKIBYOC))
	dir := t.TempDir()
	for _, item := range []struct {
		name string
		mode os.FileMode
	}{
		{"server.pem", 0o644}, {"server-key.pem", 0o600}, {"ca.pem", 0o644},
	} {
		if err := os.WriteFile(filepath.Join(dir, item.name), []byte("x"), item.mode); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BASEHARBOR_PROMETHEUS_TLS_CERT_FILE", filepath.Join(dir, "server.pem"))
	t.Setenv("BASEHARBOR_PROMETHEUS_TLS_KEY_FILE", filepath.Join(dir, "server-key.pem"))
	t.Setenv("BASEHARBOR_PROMETHEUS_TLS_TRUST_FILE", filepath.Join(dir, "ca.pem"))
	p, err := Resolve("dev", "prometheus", AuthenticationNone)
	if err != nil {
		t.Fatal(err)
	}
	if p.PKISource != PKIBYOC {
		t.Fatalf("provider override did not win: %+v", p)
	}
}

func TestManagedLocalMaterialIsUsable(t *testing.T) {
	p, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	material, err := EnsureTLSMaterial(p, t.TempDir(), "prometheus")
	if err != nil {
		t.Fatal(err)
	}
	if material.CA == "" || material.ServerCertificate == "" || material.ServerKey == "" || material.ClientCertificate == "" || material.ClientKey == "" {
		t.Fatalf("incomplete material: %+v", material)
	}
	if err := validateMaterial(material, true); err != nil {
		t.Fatalf("managed material invalid: %v", err)
	}
}

func TestResolveManagedEnvironmentRejectsNoAuthentication(t *testing.T) {
	if _, err := Resolve("prod", "prometheus", AuthenticationNone); err == nil {
		t.Fatal("managed environment accepted no authentication")
	}
}

func TestResolveTokenAuthentication(t *testing.T) {
	token := filepath.Join(t.TempDir(), "access-token")
	if err := os.WriteFile(token, []byte("opaque-test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAuthentication, string(AuthenticationToken))
	t.Setenv(EnvAuthTokenFile, token)
	p, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if p.Authentication != AuthenticationToken || p.AuthTokenFile != token {
		t.Fatalf("token authentication not resolved: %+v", p)
	}
}

func TestResolveNativeAuthenticationDoesNotAcceptGenericOverride(t *testing.T) {
	token := filepath.Join(t.TempDir(), "access-token")
	if err := os.WriteFile(token, []byte("opaque-test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAuthentication, string(AuthenticationToken))
	t.Setenv(EnvAuthTokenFile, token)
	p, err := Resolve("prod", "seaweedfs", AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	if p.Authentication != AuthenticationNative {
		t.Fatalf("native provider authentication was overridden: %+v", p)
	}
}
