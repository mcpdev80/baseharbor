package externalprovider

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
