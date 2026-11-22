package metrics

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"github.com/mcpdev80/baseharbor/internal/traces"
)

// tempoGatewayCommonName is the TLS identity the BaseHarbor Tempo gateway
// carries: the service-access certificate of the materialized provider is
// issued for the gateway service name, and Core's own Tempo read path
// (traces.tempoHTTPClient) pins the very same alias on the dial.
const tempoGatewayCommonName = "tempo-access"

// tempoAccessAlias is the backend-network alias (0428-091 P1) the datasource
// addresses and the datasource client pins as its TLS server name.
var tempoAccessAlias = traces.ManagedAccessAliases(tempoGatewayCommonName)[1]

// resolveTempoAccessAlias makes the gateway alias resolvable for the test
// process by mapping it onto the loopback address through the process-wide
// resolver hook (SetManagedGatewayAliasResolverForTests), exactly the way the
// compose-project DNS maps it onto the gateway container in a deployment.
// The datasource then dials the alias NAME with its production transport:
// verification runs against the managed certificate through the CA the policy
// trusts, and the pinned server name is the alias itself — an endpoint that
// cannot present that identity fails closed. The hook only ever maps THIS
// BaseHarbor-managed alias, never an arbitrary name.
func resolveTempoAccessAlias(t *testing.T, port int) {
	t.Helper()
	if port < 1 || port > 65535 {
		t.Fatalf("gateway stand-in port %d is not a valid TCP port", port)
	}
	restore := serviceaccess.SetManagedGatewayAliasResolverForTests(tempoGatewayCommonName, port)
	t.Cleanup(restore)
}

// mustProviderEndpoint reads the endpoint the provider state materialized.
func mustProviderEndpoint(t *testing.T, files traces.ProviderFiles) string {
	t.Helper()
	endpoint, err := traces.ProviderEndpoint(files)
	if err != nil {
		t.Fatalf("resolve the materialized Tempo endpoint: %v", err)
	}
	return endpoint
}

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

// reconcileTempoAccessGateway runs the production service-access
// reconciliation for the materialized provider (the runtime's own
// tempo-access spec, the managed-local policy of the environment), and
// returns the projected gateway material.
func reconcileTempoAccessGateway(t *testing.T, issuer *serviceissuer.Issuer, state string, m application.Manifest) serviceaccess.HTTPGatewayFiles {
	t.Helper()
	providerFiles, _, err := traces.ExistingProviderFilesAt(state, "", m)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serviceaccess.Resolve(m.Environment, string(capability.ProviderTempo), serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	gatewayFiles, err := serviceaccess.EnsureHTTPGateway(t.Context(), issuer, policy, providerFiles.Dir, traces.TempoAccessSpecForTests())
	if err != nil {
		t.Fatalf("reconcile the production tempo-access gateway: %v", err)
	}
	return gatewayFiles
}

// startTempoMaterialGateway serves the provider's reconciled gateway material
// over TLS (RequireAndVerifyClientCert, the tempo-access posture in every
// environment) with the given handler, and resolves the managed access alias
// onto the listener it starts. It is the alias-path stand-in for the managed
// gateway: the client verifies the alias-named certificate and presents the
// enrolled managed identity, exactly as in a deployment.
func startTempoMaterialGateway(t *testing.T, state string, issuer *serviceissuer.Issuer, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	gatewayFiles := reconcileTempoAccessGateway(t, issuer, state, persesTempoTraceManifest())
	serverPair, err := tls.LoadX509KeyPair(gatewayFiles.Material.ServerCertificate, gatewayFiles.Material.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := os.ReadFile(gatewayFiles.Material.CA)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(trust) {
		t.Fatal("provider CA rejected")
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{serverPair}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	resolveTempoAccessAlias(t, tempoTestServerPort(t, server.URL))
	return server
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

// startTempoMaterialGatewayWithUpstream stands the managed tempo-access
// gateway in front of a scripted Tempo search server for the manifest's
// provider: the gateway material reconciles under the manifest's own
// environment policy (a strict environment demands the enrolled identity, an
// optional-authentication environment reconciles the server pair alone), the
// listener serves exactly that material, the alias resolves onto the
// listener, and the provider endpoint is pinned to its published port. The
// bool reports whether the reconciled gateway verifies client certificates.
func startTempoMaterialGatewayWithUpstream(t *testing.T, state string, m application.Manifest, body string) (*httptest.Server, *string) {
	t.Helper()
	issuer := serviceissuer.New(t)
	upstream, seen := startTempoSearchServer(t, body, false)
	files, _, err := traces.EnsureProviderFilesAt(t.Context(), issuer, state, "", m)
	if err != nil {
		t.Fatalf("materialize Tempo provider: %v", err)
	}
	policy, err := serviceaccess.Resolve(m.Environment, string(capability.ProviderTempo), serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	// Reconcile the gateway through the runtime's own spec; the managed-local
	// PKI mints the client pair wherever the resolved policy requires
	// authentication (the strict path), and the gateway listener below
	// mirrors the reconciled posture exactly.
	gatewayFiles, err := serviceaccess.EnsureHTTPGateway(t.Context(), issuer, policy, files.Dir, traces.TempoAccessSpecForTests())
	if err != nil {
		t.Fatalf("reconcile the production tempo-access gateway: %v", err)
	}
	serverPair, err := tls.LoadX509KeyPair(gatewayFiles.Material.ServerCertificate, gatewayFiles.Material.ServerKey)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := os.ReadFile(gatewayFiles.Material.CA)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(trust) {
		t.Fatal("provider CA rejected")
	}
	clientAuth := tls.NoClientCert
	if policy.AuthenticationRequired && policy.Authentication == serviceaccess.AuthenticationMTLS {
		clientAuth = tls.RequireAndVerifyClientCert
	}
	gateway := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if clientAuth == tls.RequireAndVerifyClientCert && len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "client certificate required", http.StatusForbidden)
			return
		}
		target := upstream.URL + r.URL.Path + "?" + r.URL.RawQuery
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp, err := upstream.Client().Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	gateway.TLS = &tls.Config{Certificates: []tls.Certificate{serverPair}, ClientAuth: clientAuth, ClientCAs: pool, MinVersion: tls.VersionTLS12}
	gateway.StartTLS()
	t.Cleanup(gateway.Close)
	resolveTempoAccessAlias(t, tempoTestServerPort(t, gateway.URL))
	return gateway, seen
}
