package metrics

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

// tempoGatewayCommonName is the TLS identity the BaseHarbor Tempo gateway
// carries: the service-access certificate of the materialized provider is
// issued for the gateway service name, and Core's own Tempo read path
// (traces.tempoHTTPClient) pins the very same alias on the dial.
const tempoGatewayCommonName = "tempo-access"

// tempoTestServerPort extracts the loopback port of a test server URL.
func tempoTestServerPort(t *testing.T, raw string) int {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse test server URL %q: %v", raw, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("test server URL %q has no numeric port: %v", raw, err)
	}
	return port
}

// mustTempoProviderFiles resolves the materialized provider files of the
// provider under the state directory already pinned by BASEHARBOR_STATE_DIR.
func mustTempoProviderFiles(t *testing.T, m application.Manifest) traces.ProviderFiles {
	t.Helper()
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	files, _, err := traces.ExistingProviderFilesAt(dataDir, "", m)
	if err != nil {
		t.Fatalf("resolve the materialized Tempo provider under %s: %v", dataDir, err)
	}
	if _, err := os.Stat(files.Env); err != nil {
		t.Fatal(err)
	}
	return files
}

// mustTempoProviderClient resolves the production service-access HTTP client of
// that provider. A test without an authorized observation target must fail
// loudly, never silently skip the boundary it means to prove.
func mustTempoProviderClient(t *testing.T, m application.Manifest) *http.Client {
	t.Helper()
	client, err := tempoProviderHTTPClient(m, mustTempoProviderFiles(t, m))
	if err != nil {
		t.Fatalf("the materialized provider must yield an authorized client: %v", err)
	}
	return client
}

// useTempoServerTrust points the tempo service-access policy at the certificate
// the given test server presents, through the BYOC path Core itself supports
// (BASEHARBOR_TEMPO_SERVICE_PKI_SOURCE + _TLS_TRUST_FILE): the trust bundle
// becomes that server's own certificate, so the adapter's client verifies
// exactly the listener the datasource addresses and nothing else. The server
// name is the gateway alias Core's own Tempo read path pins.
func useTempoServerTrust(t *testing.T, leaf *x509.Certificate) {
	t.Helper()
	trust := filepath.Join(t.TempDir(), "tempo-server-trust.pem")
	if err := os.WriteFile(trust, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TEMPO_SERVICE_PKI_SOURCE", "byoc")
	t.Setenv("BASEHARBOR_TEMPO_SERVICE_TLS_TRUST_FILE", trust)
	t.Setenv("BASEHARBOR_TEMPO_SERVICE_TLS_SERVER_NAME", tempoGatewayCommonName)
}

// startTempoTLSServer runs a scripted Tempo search listener over TLS with a
// certificate issued by the provider's own test issuer for the names the
// adapter dials, so both the plain-client cells and the authorized service
// path cells address a listener the trust material actually covers.
func startTempoTLSServer(t *testing.T, issuer *serviceissuer.Issuer, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	pair := issuer.CertificateFor(t, tempoGatewayCommonName, []string{tempoGatewayCommonName, "localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")})
	tlsServer := httptest.NewUnstartedServer(handler)
	tlsServer.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	tlsServer.StartTLS()
	t.Cleanup(tlsServer.Close)
	return tlsServer
}
