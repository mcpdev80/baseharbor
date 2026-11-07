package controlplaneruntime

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
)

// brokerReadinessFiles lays out the mounts a single-default per-application
// broker binds: the runtime identity token, permissions.json,
// service-tokens.json and the AppRole credentials projection.
type brokerReadinessFiles struct {
	dir               string
	tokenFile         string
	permissionsFile   string
	serviceTokensFile string
	credentialsFile   string
}

func newBrokerReadinessFiles(t *testing.T) brokerReadinessFiles {
	t.Helper()
	f := brokerReadinessFiles{dir: t.TempDir()}
	f.tokenFile = filepath.Join(f.dir, "runtime-token")
	if err := os.WriteFile(f.tokenFile, []byte("0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.permissionsFile = filepath.Join(f.dir, "permissions.json")
	if err := os.WriteFile(f.permissionsFile, []byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.serviceTokensFile = filepath.Join(f.dir, "service-tokens.json")
	if err := os.WriteFile(f.serviceTokensFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.credentialsFile = filepath.Join(f.dir, "openbao-credentials")
	if err := os.WriteFile(f.credentialsFile, []byte("OPENBAO_ROLE_ID=role-1\nOPENBAO_SECRET_ID=secret-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func brokerReadinessConfig(f brokerReadinessFiles) Config {
	return Config{
		RuntimeAppName:           "demo-app",
		RuntimeEnvironment:       "dev",
		RuntimeTokenFile:         f.tokenFile,
		RuntimePermissionsFile:   f.permissionsFile,
		RuntimeServiceTokensFile: f.serviceTokensFile,
		RuntimeCredentialsFile:   f.credentialsFile,
	}
}

// appRoleLoginHandler is the OpenBao stand-in used by the readiness tests:
// the exact witnesses the w269/vm-rebuild runs reported from the real broker
// (initialized, sealed:false, working AppRole login).
func appRoleLoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sys/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"initialized":true,"sealed":false}`))
		case "/v1/auth/approle/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"auth":{"client_token":"readiness-test-token"}}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func newBaoLoginClient(t *testing.T, handler http.Handler) *openbao.ApplicationRuntimeClient {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	caFile := filepath.Join(t.TempDir(), "bao-ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o644); err != nil {
		t.Fatal(err)
	}
	client, err := openbao.NewApplicationRuntimeClientWithCA(server.URL, caFile)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// executorFixture is a runtime executor stand-in with a real test PKI: the
// server requires and verifies a client certificate, exactly like the
// production executor. The readiness client is constructed through the
// production constructor with this material; only ephemeral-CA trust is
// supplied through the narrow transport override.
type executorFixture struct {
	server    *httptest.Server
	caFile    string
	clientKey *rsa.PrivateKey
	clientDER []byte
}

func newExecutorFixture(t *testing.T, ready func() bool) *executorFixture {
	t.Helper()
	dir := t.TempDir()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "baseharbor-readiness-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o644); err != nil {
		t.Fatal(err)
	}

	writeLeaf := func(name string, dnsNames []string, workloadURI string) (tls.Certificate, []byte, *rsa.PrivateKey) {
		t.Helper()
		leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			t.Fatal(err)
		}
		template := x509.Certificate{
			SerialNumber: serial,
			Subject:      pkix.Name{CommonName: name},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			DNSNames:     dnsNames,
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		}
		if workloadURI != "" {
			parsed, err := url.Parse(workloadURI)
			if err != nil {
				t.Fatal(err)
			}
			template.URIs = []*url.URL{parsed}
		}
		der, err := x509.CreateCertificate(rand.Reader, &template, caCert, &leafKey.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der}, Leaf: mustParse(der), PrivateKey: leafKey}, der, leafKey
	}

	serverTLS, _, _ := writeLeaf("executor-server", []string{"127.0.0.1", "localhost"}, "")
	clientTLSCert, clientDER, clientKey := writeLeaf("probe-client", nil, "spiffe://baseharbor/apps/demo-app/dev")

	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		if len(r.TLS.PeerCertificates) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverTLS},
		ClientAuth:   tls.RequireAnyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
		// RequireAnyClientCert keeps the handshake working for clients with or
		// without an identity so the handler can assert certificate presence
		// on the exact Check route; chain verification is not what the
		// readiness cells prove.
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	_ = clientTLSCert
	return &executorFixture{server: server, caFile: caFile, clientKey: clientKey, clientDER: clientDER}
}

func mustParse(der []byte) *x509.Certificate {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	return cert
}

// client returns the executor readiness client built with the production
// constructor and the full identity material, with the httptest trust
// override for the ephemeral listener.
func (f *executorFixture) client(t *testing.T) *runtimeexecutor.Client {
	t.Helper()
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client-cert.pem")
	keyPath := filepath.Join(dir, "client-key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.clientDER}), 0o644); err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(f.clientKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := runtimeexecutor.NewClient(runtimeexecutor.ClientConfig{
		URL:      f.server.URL,
		CAFile:   f.caFile,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Build the transport from the SAME material the production constructor
	// just loaded and verified, and add only the trust fact an ephemeral
	// httptest listener forces: the ServerName SNI pin. Certificate identity
	// and CA validation come from the production files, nothing else changes.
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(f.caFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("test CA is invalid")
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      roots,
		Certificates: []tls.Certificate{cert},
		ServerName:   "localhost",
	}}
	runtimeexecutor.ExecutorTransportOverrideForTest(client, tr)
	return client
}

// TestBoundBrokerReadyzFailsClosedWithoutExecutorConfig proves the real
// readiness path fails closed with the runtime-executor dependency named when
// the broker received no executor configuration at all (no URL, no client):
// this is the permanent, cause-less not_ready the published 0.4.26 edge image
// showed on the VMs. The probe must name its dependency and must never be
// waived implicitly. A configured-but-unbuildable client (URL set, client
// nil — a projection that disappeared after startup) must land in the same
// fail-closed branch, never the waiver.
func TestBoundBrokerReadyzFailsClosedWithoutExecutorConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
	}{
		{name: "no-executor-configuration", url: ""},
		{name: "configured-client-unbuildable", url: "https://baseharbor-runtime-executor:9443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBrokerReadinessFiles(t)
			cfg := brokerReadinessConfig(f)
			cfg.RuntimeExecutorURL = tc.url

			mux := http.NewServeMux()
			registerHealthHandlers(mux, cfg, serverDependencies{})

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body["status"] != "not_ready" {
				t.Fatalf("status field = %q, want not_ready", body["status"])
			}
			if body["dependency"] != "runtime-executor" {
				t.Fatalf("dependency field = %q, want runtime-executor; a cause-less not_ready is the gap-2 defect", body["dependency"])
			}
		})
	}
}

// TestBoundBrokerReadyzGreenWithAllPresentDependencies reproduces the gap-2
// GREEN half: a bound broker whose declared dependencies are all present —
// OpenBao reachable through the AppRole client and the mTLS executor client
// answering ready — must report status ready even though no database is
// attached (the broker has none). Readiness is computed from the checks that
// exist, never pinned to a phantom failure.
func TestBoundBrokerReadyzGreenWithAllPresentDependencies(t *testing.T) {
	f := newBrokerReadinessFiles(t)
	cfg := brokerReadinessConfig(f)

	baoClient := newBaoLoginClient(t, appRoleLoginHandler())
	executor := newExecutorFixture(t, func() bool { return true })
	executorClient := executor.client(t)
	cfg.RuntimeExecutorURL = executor.server.URL

	deps := serverDependencies{boundRuntimeClient: baoClient, boundExecutorClient: executorClient}
	mux := http.NewServeMux()
	registerHealthHandlers(mux, cfg, deps)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("a broker with all dependencies present must be READY, got %d body %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ready" {
		t.Fatalf("status = %q, want ready", body["status"])
	}
}

// TestBoundBrokerReadyzRedWhenExecutorReadyzFails proves the RED half: when
// the executor dependency is actually down, readiness must stay fail-closed
// and name it; the fix never fabricates green — not even with the explicit
// waiver on, because the waiver is inert once a client exists.
func TestBoundBrokerReadyzRedWhenExecutorReadyzFails(t *testing.T) {
	f := newBrokerReadinessFiles(t)
	cfg := brokerReadinessConfig(f)

	var ready atomic.Bool
	executor := newExecutorFixture(t, func() bool { return ready.Load() })
	executorClient := executor.client(t)
	cfg.RuntimeExecutorURL = executor.server.URL
	t.Setenv("BASEHARBOR_RUNTIME_EXECUTOR_NOT_REQUIRED", "true")

	deps := serverDependencies{boundExecutorClient: executorClient}
	mux := http.NewServeMux()
	registerHealthHandlers(mux, cfg, deps)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("a down executor must stay red even with the waiver on, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["dependency"] != "runtime-executor" {
		t.Fatalf("body = %v, want dependency runtime-executor", body)
	}
}

// TestBoundBrokerReadyzWaivesAbsentExecutorOnlyByExplicitOptIn proves the
// legacy-image escape hatch is explicit and off by default: a broker that
// received no executor URL and no client blocks readiness fail-closed until
// the operator sets BASEHARBOR_RUNTIME_EXECUTOR_NOT_REQUIRED=true. The
// switch is re-read on every probe, so recovery needs no broker restart.
func TestBoundBrokerReadyzWaivesAbsentExecutorOnlyByExplicitOptIn(t *testing.T) {
	f := newBrokerReadinessFiles(t)
	cfg := brokerReadinessConfig(f)
	baoClient := newBaoLoginClient(t, appRoleLoginHandler())

	mux := http.NewServeMux()
	registerHealthHandlers(mux, cfg, serverDependencies{boundRuntimeClient: baoClient})

	t.Setenv("BASEHARBOR_RUNTIME_EXECUTOR_NOT_REQUIRED", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("without the opt-in the broker must stay fail-closed, got %d", rec.Code)
	}

	t.Setenv("BASEHARBOR_RUNTIME_EXECUTOR_NOT_REQUIRED", "true")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("with the explicit waiver readiness must report the remaining dependencies, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ready" {
		t.Fatalf("status = %q, want ready", body["status"])
	}
}

// TestBoundBrokerReadyzExecutorRecoveryTurnsGreenWithoutRestart proves the
// permanent-not_ready behaviour observed on the VMs is broken: the same
// running server flips to READY the moment its executor dependency becomes
// reachable, with no process restart.
func TestBoundBrokerReadyzExecutorRecoveryTurnsGreenWithoutRestart(t *testing.T) {
	f := newBrokerReadinessFiles(t)
	cfg := brokerReadinessConfig(f)

	var ready atomic.Bool
	executor := newExecutorFixture(t, func() bool { return ready.Load() })
	executorClient := executor.client(t)
	cfg.RuntimeExecutorURL = executor.server.URL

	mux := http.NewServeMux()
	registerHealthHandlers(mux, cfg, serverDependencies{boundExecutorClient: executorClient})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("while the executor is down the broker must be red, got %d", rec.Code)
	}

	ready.Store(true)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("after executor recovery the broker must turn green without a restart, got %d", rec.Code)
	}
}

// TestBoundBrokerReadyzRedWhenOpenBaoLoginFails proves the openbao branch of
// the same readiness path names its dependency when AppRole login is
// rejected, the class of permanent not_ready a stale AppRole secret-id
// projection causes.
func TestBoundBrokerReadyzRedWhenOpenBaoLoginFails(t *testing.T) {
	f := newBrokerReadinessFiles(t)
	cfg := brokerReadinessConfig(f)
	baoClient := newBaoLoginClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	mux := http.NewServeMux()
	registerHealthHandlers(mux, cfg, serverDependencies{boundRuntimeClient: baoClient})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("a rejected AppRole login must stay red, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["dependency"] != "openbao" {
		t.Fatalf("body = %v, want dependency openbao", body)
	}
}
