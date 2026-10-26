package serviceaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActualLocalIssuerGatewaySANReconciliationPreservesAuthority(t *testing.T) {
	issuer, err := EnsureLocalIssuer(filepath.Join(t.TempDir(), "issuer"))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := Resolve("dev", "dev-gateway", AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	policy.ServerName = "auth.baha.localhost"
	materialDir := filepath.Join(t.TempDir(), "gateway-pki")
	first, err := EnsureTLSMaterial(t.Context(), issuer, policy, materialDir, "auth.baha.localhost")
	if err != nil {
		t.Fatal(err)
	}
	originalCA, err := os.ReadFile(first.CA)
	if err != nil {
		t.Fatal(err)
	}
	originalAuthority, err := os.ReadFile(filepath.Join(issuer.Dir, "issuer.pem"))
	if err != nil {
		t.Fatal(err)
	}
	firstCertificate, err := os.ReadFile(first.ServerCertificate)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsureTLSMaterial(t.Context(), issuer, policy, materialDir, "auth.baha.localhost", "app.baha.localhost")
	if err != nil {
		t.Fatalf("same owned gateway could not expand SANs through actual issuer: %v", err)
	}
	secondCertificate, err := os.ReadFile(second.ServerCertificate)
	if err != nil || string(firstCertificate) == string(secondCertificate) {
		t.Fatal("changed SANs were not reconciled")
	}
	afterCA, err := os.ReadFile(second.CA)
	if err != nil || string(afterCA) != string(originalCA) {
		t.Fatal("SAN reconciliation replaced public trust")
	}
	afterAuthority, err := os.ReadFile(filepath.Join(issuer.Dir, "issuer.pem"))
	if err != nil || string(afterAuthority) != string(originalAuthority) {
		t.Fatal("SAN reconciliation changed private Target authority")
	}
	certificate, err := readCertificate(second.ServerCertificate)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"auth.baha.localhost", "app.baha.localhost"} {
		if err := certificate.VerifyHostname(host); err != nil {
			t.Fatal("reconciled SAN missing")
		}
	}
	// Complete an actual TLS handshake for the newly added callback host using
	// the unchanged public trust, rather than trusting certificate metadata.
	certPEM, err := os.ReadFile(second.ServerCertificate)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(second.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(originalCA) {
		t.Fatal("original trust invalid")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "app.baha.localhost"}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second * 5}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal("actual callback TLS failed")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("actual callback HTTP failed")
	}
	third, err := EnsureTLSMaterial(t.Context(), issuer, policy, materialDir, "app.baha.localhost", "auth.baha.localhost")
	if err != nil {
		t.Fatal(err)
	}
	stable, err := os.ReadFile(third.ServerCertificate)
	if err != nil || string(stable) != string(secondCertificate) {
		t.Fatal("unchanged gateway names reissued material")
	}
}

func TestActualLocalGatewayReconciliationRejectsForeignAndRevokedLeaves(t *testing.T) {
	for _, scenario := range []string{"foreign-leaf", "revoked-leaf"} {
		t.Run(scenario, func(t *testing.T) {
			issuer, err := EnsureLocalIssuer(filepath.Join(t.TempDir(), "issuer"))
			if err != nil {
				t.Fatal(err)
			}
			policy, err := Resolve("dev", "dev-gateway", AuthenticationNative)
			if err != nil {
				t.Fatal(err)
			}
			policy.ServerName = "auth.baha.localhost"
			dir := filepath.Join(t.TempDir(), "gateway-pki")
			material, err := EnsureTLSMaterial(t.Context(), issuer, policy, dir, "auth.baha.localhost")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "foreign-leaf" {
				foreign, err := EnsureLocalIssuer(filepath.Join(t.TempDir(), "foreign"))
				if err != nil {
					t.Fatal(err)
				}
				leaf, err := foreign.Issue(context.Background(), CertificateRequest{CommonName: policy.ServerName, DNSNames: []string{policy.ServerName, "localhost"}, TTL: time.Hour})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(material.ServerCertificate, leaf.Certificate, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(material.ServerKey, leaf.PrivateKey, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(material.ServerCertificate)
				if err != nil {
					t.Fatal(err)
				}
				block, _ := pem.Decode(data)
				if block == nil {
					t.Fatal("leaf invalid")
				}
				leaf, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					t.Fatal(err)
				}
				if err := issuer.Revoke(t.Context(), leaf.SerialNumber.Text(16)); err != nil {
					t.Fatal(err)
				}
			}
			paths := []string{material.CA, material.ServerCertificate, material.ServerKey, filepath.Join(dir, "state.json"), filepath.Join(issuer.Dir, "issuer.pem")}
			before := map[string][]byte{}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = data
			}
			if _, err := EnsureTLSMaterial(t.Context(), issuer, policy, dir, "auth.baha.localhost", "app.baha.localhost"); err == nil {
				t.Fatal("foreign or revoked certificate renewed")
			}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != string(before[path]) {
					t.Fatal("failed renewal changed retained recovery material")
				}
			}
		})
	}
}
