package operatorauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// writeTestCA writes a self-signed CA certificate to path and returns its PEM
// as well as the parsed certificate and signing key so callers can mint leaf
// certificates for TLS test servers.
func writeTestCA(t *testing.T, path string) (caPEM []byte, cert *x509.Certificate, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Managed CA"},
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(24, 0, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, caPEM, 0o600); err != nil {
		t.Fatalf("write test CA file %s: %v", path, err)
	}
	return caPEM, cert, key
}

func TestResolveTrustFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "explicit.pem")
	envPath := filepath.Join(dir, "env.pem")

	// No files at all: falls through to ErrNoOperatorTrust with the remedy path
	// naming the state-dir default file.
	if _, path, err := ResolveTrustFile(TrustOptions{ExplicitCaFile: filepath.Join(dir, "does-not-exist.pem")}); !errors.Is(err, ErrNoOperatorTrust) {
		t.Fatalf("expected ErrNoOperatorTrust when no CA is available, got %v", err)
	} else if path != DefaultTrustPath() {
		t.Fatalf("expected remedy path to name the default trust file %q, got %q", DefaultTrustPath(), path)
	}

	// Explicit file wins over the env file even when the env file would also
	// resolve.
	if _, _, caKeyExplicit := writeTestCA(t, explicit); caKeyExplicit == nil {
		t.Fatal("explicit CA key is nil")
	}
	if _, _, caKeyEnv := writeTestCA(t, envPath); caKeyEnv == nil {
		t.Fatal("env CA key is nil")
	}
	t.Setenv(EnvTrustFile, envPath)
	data, path, err := ResolveTrustFile(TrustOptions{ExplicitCaFile: explicit})
	if err != nil {
		t.Fatalf("explicit CA should resolve: %v", err)
	}
	if path != explicit {
		t.Fatalf("explicit CA should win over env file, got path %q", path)
	}
	if len(data) == 0 {
		t.Fatal("resolved CA is empty")
	}
}

func TestDiscoveryClientAdditiveRoots(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "managed-ca.pem")
	_, caCert, caKey := writeTestCA(t, caPath)

	// Serve a discovery document over TLS with a leaf signed by the test CA so
	// the only way to verify it is the additive managed-CA root pool.
	leafKey, leafErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if leafErr != nil {
		t.Fatalf("generate leaf key: %v", leafErr)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		DNSNames:     []string{"127.0.0.1"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
	}
	leafDER, leafErr := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if leafErr != nil {
		t.Fatalf("generate leaf certificate: %v", leafErr)
	}
	leafKeyPEM, marshalErr := x509.MarshalPKCS8PrivateKey(leafKey)
	if marshalErr != nil {
		t.Fatalf("marshal leaf key: %v", marshalErr)
	}
	leafCert, x509Err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyPEM}),
	)
	if x509Err != nil {
		t.Fatalf("build leaf key pair: %v", x509Err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"https://issuer.example.test","jwks_uri":"https://example.test/jwks"}`))
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{leafCert}}
	server.StartTLS()
	defer server.Close()
	_ = server

	t.Setenv(EnvTrustFile, caPath)
	client, err := discoveryClient(t.Context(), Config{Issuer: "https://issuer.example.test"}, TrustOptions{})
	if err != nil {
		t.Fatalf("discoveryClient with managed CA should succeed: %v", err)
	}
	if client == nil {
		t.Fatal("discoveryClient returned nil when a managed CA was configured")
	}
	// The client must reach the TLS server using the managed CA.
	resp, err := client.Get(server.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("discovery request against managed-CA TLS server failed (trust pool not additive): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d from managed-CA discovery", resp.StatusCode)
	}
}

func TestDiscoveryClientNoExplicitCAKeepsDefaultTransport(t *testing.T) {
	client, err := discoveryClient(t.Context(), Config{Issuer: "https://example.test"}, TrustOptions{})
	if err != nil {
		t.Fatalf("no explicit CA must not error, got %v", err)
	}
	if client != nil {
		t.Fatal("expected the default transport (nil client) when no CA is configured")
	}
}

func TestDiscoveryClientRejectsNonCertificate(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "not-a-ca.txt")
	if err := os.WriteFile(bad, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvTrustFile, bad)
	if _, err := discoveryClient(t.Context(), Config{Issuer: "https://example.test"}, TrustOptions{}); !errors.Is(err, ErrNoOperatorTrust) {
		t.Fatalf("expected ErrNoOperatorTrust for a non-certificate CA file, got %v", err)
	}
}

// _ guards the oidc import used by headless completion in login_test.go.
var _ = oidc.ClientContext
