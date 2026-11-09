package controlplaneruntime

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// managedCAOIDCIssuer starts an OIDC discovery endpoint whose TLS is served by a
// CA that is deliberately absent from the host system trust pool, exactly like
// the CA a managed Core bootstraps. It returns the issuer URL and the CA file
// path so tests can exercise the additive operator trust path.
func managedCAOIDCIssuer(t *testing.T) (string, string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "baseharbor-managed-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "managed-operator-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "managed-oidc"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	var issuer string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 issuer,
				"authorization_endpoint": issuer + "/authorize",
				"token_endpoint":         issuer + "/token",
				"jwks_uri":               issuer + "/keys",
			})
		case strings.HasSuffix(r.URL.Path, "/keys"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}}}
	server.StartTLS()
	issuer = server.URL + "/realms/bh-operators-dev"
	t.Cleanup(server.Close)
	return issuer, caPath
}

// TestOperatorOIDCVerifierTrustsManagedCA proves the operator API verifies a
// managed-Keycloak-style issuer through the additive managed CA. The CA is not
// installed into any system trust store, so success is only possible when
// cfg.OperatorOIDCTrustFile reaches auth.Config.TrustFile.
func TestOperatorOIDCVerifierTrustsManagedCA(t *testing.T) {
	issuer, caPath := managedCAOIDCIssuer(t)
	verifier, err := operatorOIDCVerifier(context.Background(), Config{
		OIDCIssuer:            issuer,
		OIDCAudiences:         []string{"baseharbor-cli"},
		OperatorOIDCTrustFile: caPath,
	})
	if err != nil {
		t.Fatalf("managed operator CA must verify the managed issuer: %v", err)
	}
	if verifier == nil {
		t.Fatal("verifier must be constructed")
	}
}

// TestOperatorOIDCVerifierFailsClosedWithoutManagedCA is the negative control:
// with the same issuer and no managed CA the discovery must fail on certificate
// verification. Trust is never disabled and the system pool is never replaced.
func TestOperatorOIDCVerifierFailsClosedWithoutManagedCA(t *testing.T) {
	issuer, _ := managedCAOIDCIssuer(t)
	_, err := operatorOIDCVerifier(context.Background(), Config{
		OIDCIssuer:    issuer,
		OIDCAudiences: []string{"baseharbor-cli"},
	})
	if err == nil {
		t.Fatal("untrusted managed issuer must fail closed")
	}
	if !strings.Contains(err.Error(), "certificate signed by unknown authority") &&
		!strings.Contains(err.Error(), "unknown authority") &&
		!strings.Contains(err.Error(), "x509") {
		t.Fatalf("expected an x509 trust failure, got: %v", err)
	}
}
