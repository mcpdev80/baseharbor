package externalprovider

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestVerifyOIDCUsesConfiguredTrustAndDiscovery(t *testing.T) {
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"` + issuer + `","authorization_endpoint":"` + issuer + `/authorize","token_endpoint":"` + issuer + `/token","jwks_uri":"` + issuer + `/jwks"}`))
	}))
	defer server.Close()
	issuer = server.URL

	cert := server.Certificate()
	if cert == nil {
		t.Fatal("TLS server certificate missing")
	}
	caPath := filepath.Join(t.TempDir(), "server-ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(caPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := x509.ParseCertificate(cert.Raw); err != nil {
		t.Fatal(err)
	}

	reg := Registration{
		ID:         "company-idp",
		ProviderID: "company/oidc",
		Endpoint:   server.URL,
		Provider: capability.Provider{
			Kind:         capability.ProviderKind("company-oidc"),
			Capabilities: []capability.Kind{capability.Identity},
		},
		Trust: Trust{Mode: TrustCustomCA, CAReference: caPath},
	}
	got, err := Verify(t.Context(), reg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Semantic != "ok" || got.Capability != string(capability.Identity) || got.TLS != "ok" {
		t.Fatalf("verification=%#v", got)
	}
}

func TestExternalOIDCDiscoveryRejectsUntrustedIssuerAndCredentialEndpoints(t *testing.T) {
	for _, scenario := range []string{"issuer", "userinfo", "query", "oversize", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			var issuer string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "redirect" {
					http.Redirect(w, r, "https://private-sentinel@unrelated.example.test", http.StatusFound)
					return
				}
				if scenario == "oversize" {
					_, _ = w.Write(bytes.Repeat([]byte("x"), maxMaterialBytes+1))
					return
				}
				doc := map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks"}
				switch scenario {
				case "issuer":
					doc["issuer"] = "https://unrelated.example.test"
				case "userinfo":
					doc["token_endpoint"] = "https://client:private-sentinel@unrelated.example.test/token"
				case "query":
					doc["token_endpoint"] = issuer + "/token?client_secret=private-sentinel"
				}
				_ = json.NewEncoder(w).Encode(doc)
			}))
			defer server.Close()
			issuer = server.URL
			ca := filepath.Join(t.TempDir(), "ca.pem")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0644); err != nil {
				t.Fatal(err)
			}
			reg := Registration{ID: "issuer", ProviderID: "company/oidc", Endpoint: issuer, Provider: capability.ExternalOIDC, Trust: Trust{Mode: TrustCustomCA, CAReference: ca}}
			if _, err := Verify(t.Context(), reg); err == nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Fatal("unverified or credential-bearing discovery accepted or disclosed")
			}
		})
	}
}

func TestExternalMTLSAuthenticatesWithProtectedMaterial(t *testing.T) {
	dir := t.TempDir()
	ca, key := writeTestCA(t, dir)
	writeTestLeaf(t, dir, "consumer", ca, key, []string{"consumer.example.test"}, x509.ExtKeyUsageClientAuth)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	var issuer string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.VerifiedChains) == 0 {
			t.Error("consumer mTLS identity was not verified")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks"})
	}))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	issuer = server.URL
	serverCA := filepath.Join(dir, "server-ca.pem")
	if err := os.WriteFile(serverCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0644); err != nil {
		t.Fatal(err)
	}
	reg := Registration{ID: "mtls-issuer", ProviderID: "company/oidc", Endpoint: issuer, Provider: capability.ExternalOIDC, Trust: Trust{Mode: TrustMTLS, CAReference: serverCA, ClientCertificate: filepath.Join(dir, "consumer.pem"), ClientKey: filepath.Join(dir, "consumer.key")}}
	if _, err := Verify(t.Context(), reg); err != nil {
		t.Fatal("actual protected mTLS consumer authentication failed", err)
	}
	if err := os.Chmod(reg.Trust.ClientKey, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(t.Context(), reg); err == nil {
		t.Fatal("publicly readable private key retained authentication authority")
	}
}
