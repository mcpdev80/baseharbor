package serviceaccess

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	material, err := EnsureTLSMaterial(context.Background(), newTestIssuer(t), p, t.TempDir(), "prometheus")
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

func TestManagedLocalMaterialRenewsBeforeExpiry(t *testing.T) {
	p, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	issuer := newTestIssuer(t)
	issuer.validity = 48 * time.Hour
	dir := t.TempDir()

	if _, err := EnsureTLSMaterial(context.Background(), issuer, p, dir, "prometheus"); err != nil {
		t.Fatal(err)
	}
	if issuer.renewCalls != 0 {
		t.Fatalf("initial issuance unexpectedly renewed: %d", issuer.renewCalls)
	}
	if _, err := EnsureTLSMaterial(context.Background(), issuer, p, dir, "prometheus"); err != nil {
		t.Fatal(err)
	}
	if issuer.renewCalls == 0 {
		t.Fatal("near-expiry managed certificate was not renewed through Issuer.Renew")
	}

	state, err := readManagedPKIState(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 2 || state.LifecycleOwner != "issuer" || state.RenewalMode != "automatic-reconcile" {
		t.Fatalf("unexpected managed PKI lifecycle state: %+v", state)
	}
}

func TestManagedLocalRootRotationReissuesLeaf(t *testing.T) {
	p, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	first := newTestIssuer(t)
	if _, err := EnsureTLSMaterial(context.Background(), first, p, dir, "prometheus"); err != nil {
		t.Fatal(err)
	}

	rotated := newTestIssuer(t)
	if _, err := EnsureTLSMaterial(context.Background(), rotated, p, dir, "prometheus"); err != nil {
		t.Fatal(err)
	}
	if rotated.issueCalls == 0 {
		t.Fatal("issuer root rotation did not force replacement issuance")
	}
	trust, err := rotated.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !trustBundleMatchesFile(filepath.Join(dir, "ca.pem"), trust.PEM) {
		t.Fatal("rotated issuer trust bundle was not projected")
	}
}

func TestResolveIssuerBackedExternalPKI(t *testing.T) {
	t.Setenv(EnvPKISource, string(PKIExternal))
	t.Setenv(EnvIssuerRef, "test://issuer")
	p, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if p.PKISource != PKIExternal || p.IssuerReference != "test://issuer" {
		t.Fatalf("unexpected external issuer policy: %+v", p)
	}
	material, err := EnsureTLSMaterial(context.Background(), newTestIssuer(t), p, t.TempDir(), "prometheus")
	if err != nil {
		t.Fatal(err)
	}
	if material.Source != PKIExternal {
		t.Fatalf("material source = %q, want %q", material.Source, PKIExternal)
	}
}

func TestIssuerBackedExternalPKIRejectsMismatchedAdapter(t *testing.T) {
	t.Setenv(EnvPKISource, string(PKIExternal))
	t.Setenv(EnvIssuerRef, "enterprise://issuer")
	p, err := Resolve("prod", "prometheus", AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureTLSMaterial(context.Background(), newTestIssuer(t), p, t.TempDir(), "prometheus"); err == nil {
		t.Fatal("mismatched external issuer adapter was accepted")
	}
}

func TestBYOCRejectsIssuerAdapter(t *testing.T) {
	t.Setenv(EnvPKISource, string(PKIBYOC))
	t.Setenv(EnvIssuerRef, "test://issuer")
	if _, err := Resolve("prod", "prometheus", AuthenticationMTLS); err == nil {
		t.Fatal("BYOC unexpectedly accepted issuer ownership")
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
