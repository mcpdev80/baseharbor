package runtimeexecutor

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

// checkReachedRecorder is an http.Handler for the /readyz route that records,
// via an atomic flag, that it was entered. It stands in for the readyz handler
// Client.Check() drives. Because Check() routes to /readyz (not to the
// /internal/ execute handler), asserting this flag — independently of the
// execute-path flag — proves the negative/positive check observation is on the
// exact route Check drives, not on an unrelated handler.
type checkReachedRecorder struct {
	entered atomic.Bool
}

func (r *checkReachedRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.entered.Store(true)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// executeCountingHandler is a TEST-ONLY wrapper around the production Handler.
//
// Production handler.go must never carry test instrumentation (no
// HandlerReached counter), so it cannot prove that the /internal/ execute route
// was never entered on a refused handshake. Because the execute route is
// registered as a prefix handler (mux.Handle("/internal/", handler)), a plain
// *Handler value cannot be handed to the mux without losing the ability to
// observe whether it ran. This wrapper keeps the real *Handler (so behavior,
// routing and request handling are identical to production) while exposing a
// per-path atomic flag that flips true the moment ServeHTTP is entered.
//
// Tests observe wrapper.reached.Load() — the execute-path counterpart to the
// independent checkReachedRecorder.entered flag — so the two routes are never
// conflated.
type executeCountingHandler struct {
	reached atomic.Bool
	inner   *Handler
}

func (w *executeCountingHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	w.reached.Store(true)
	w.inner.ServeHTTP(resp, req)
}

// newExecuteCountingHandler wraps a freshly constructed production Handler so a
// test can observe whether the execute route was entered without polluting the
// production Handler with any test counter.
func newExecuteCountingHandler(t *testing.T) *executeCountingHandler {
	t.Helper()
	h, err := NewHandler(fakeResources{})
	if err != nil {
		t.Fatal(err)
	}
	return &executeCountingHandler{inner: h}
}

// assertTLSHandshakeCause fails the test unless err is a TLS/handshake/
// verification failure, classified by type. It accepts exactly three concrete
// TLS-stack shapes: an *x509.UnknownAuthorityError or *x509.CertificateInvalidError
// (server-side chain verification), a wrapped *tls.AlertError (an alert the
// peer sent), or a *net.OpError whose Op is "remote error" (the server-side
// client-cert refusal alert, e.g. "certificate required" / "unknown certificate
// authority"). Pure substring tolerance on "tls"|"x509"|"certificate" is
// deliberately NOT used, so the negative documents a cryptographic/verification
// failure and not some unrelated transport error.
func assertTLSHandshakeCause(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a TLS/handshake/verification error, got nil")
	}
	var unknownAuthority *x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return
	}
	var certificateInvalid *x509.CertificateInvalidError
	if errors.As(err, &certificateInvalid) {
		return
	}
	var alert *tls.AlertError
	if errors.As(err, &alert) {
		return
	}
	var certVerify *tls.CertificateVerificationError
	if errors.As(err, &certVerify) {
		return
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "remote error" {
		// On a *net.OpError with Op=="remote error" the refusal alert is wrapped
		// one or two levels deeper. Classify by the INNER cause's TLS/x509-tauglichkeit,
		// not merely by the Op-zeichenkette: a real client-cert refusal surfaces as
		// *net.OpError{Op:"read", Err: <tls alert/verification>} or a concrete
		// crypto/tls or crypto/x509 error, whereas a fabricated non-TLS error
		// (e.g. a plain *errors.errorString) must be REJECTED.
		var alertErr *tls.AlertError
		var recordHeader *tls.RecordHeaderError
		var certVerify *tls.CertificateVerificationError
		var unknownAuthority *x509.UnknownAuthorityError
		var certInvalid *x509.CertificateInvalidError
		if errors.As(err, &alertErr) ||
			errors.As(err, &recordHeader) ||
			errors.As(err, &certVerify) ||
			errors.As(err, &unknownAuthority) ||
			errors.As(err, &certInvalid) {
			return
		}
		// The inner error of a real client-cert refusal is a crypto/tls transport
		// error. In go1.26 the concrete refusal type is the unexported
		// tls.alert (wrapping the "tls: certificate required" message); older
		// versions export *tls.AlertError, *tls.RecordHeaderError or
		// *tls.CertificateVerificationError. The discriminator is that the inner
		// error's dynamic type belongs to the crypto/tls package (its %T string
		// carries the "tls." qualifier) — NOT a bare *errors.errorString such as
		// the fabricated "not a TLS error".
		if opErr.Err != nil && isTLSInnerType(opErr.Err) {
			return
		}
	}
	t.Fatalf("error %q is not a classified TLS/handshake/verification cause", err.Error())
}

// isTLSInnerType reports whether err is a crypto/tls transport-layer refusal
// (e.g. the unexported tls.alert, or an exported *tls.AlertError /
// *tls.RecordHeaderError / *tls.CertificateVerificationError) or a crypto/x509
// verification failure. A bare *errors.errorString (the fabricated non-TLS
// case) is rejected.
func isTLSInnerType(err error) bool {
	if err == nil {
		return false
	}
	var alertErr *tls.AlertError
	var recordHeader *tls.RecordHeaderError
	var certVerify *tls.CertificateVerificationError
	var unknownAuthority *x509.UnknownAuthorityError
	var certInvalid *x509.CertificateInvalidError
	if errors.As(err, &alertErr) ||
		errors.As(err, &recordHeader) ||
		errors.As(err, &certVerify) ||
		errors.As(err, &unknownAuthority) ||
		errors.As(err, &certInvalid) {
		return true
	}
	// crypto/tls refuses the handshake before dispatch; its refusal type carries
	// the "tls." qualifier in its fully-qualified name (e.g. *tls.alert), which
	// distinguishes it from an unrelated transport error.
	typeName := fmt.Sprintf("%T", err)
	for _, name := range []string{
		"tls.alert",
		"tls.AlertError",
		"tls.RecordHeaderError",
		"tls.CertificateVerificationError",
		"x509.UnknownAuthorityError",
		"x509.CertificateInvalidError",
		"x509.UnhandledCriticalExtension",
	} {
		if strings.Contains(typeName, name) {
			return true
		}
	}
	return false
}

// testCA is a throwaway PKI used to drive a real mutual-TLS handshake against a
// local runtime executor server. It is NOT wired to any production trust anchor.
type testCA struct {
	dir            string
	caCertPEM      []byte
	caKeyPEM       []byte
	caCert         *x509.Certificate
	caKey          *ecdsa.PrivateKey
	serverCert     []byte
	serverKey      []byte
	caCertPath     string
	serverCertPath string
	serverKeyPath  string
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	dir := t.TempDir()
	ca := &testCA{dir: dir}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "baseharbor-runtime-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	ca.caCert = caCert
	ca.caKey = caKey
	ca.caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	ca.caKeyPEM, err = x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		t.Fatal(err)
	}

	// Server certificate signed by the test CA.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "runtime-executor"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := x509.ParseCertificate(serverDER)
	if err != nil {
		t.Fatal(err)
	}
	ca.serverCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw})
	ca.serverKey, err = x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}

	ca.write(t, "ca.pem", ca.caCertPEM, 0o644)
	ca.write(t, "server-cert.pem", ca.serverCert, 0o644)
	ca.write(t, "server-key.pem", ca.serverKey, 0o600)
	ca.caCertPath = filepath.Join(dir, "ca.pem")
	ca.serverCertPath = filepath.Join(dir, "server-cert.pem")
	ca.serverKeyPath = filepath.Join(dir, "server-key.pem")
	return ca
}

func (c *testCA) write(t *testing.T, name string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(c.dir, name), data, mode); err != nil {
		t.Fatal(err)
	}
}

// workloadClientMaterial is a client identity (certificate + key) signed by the
// test CA carrying a valid BaseHarbor workload SPIFFE URI.
type workloadClientMaterial struct {
	certPEM []byte
	keyPEM  []byte
}

func (c *testCA) workloadClient(t *testing.T, spiffe string) *workloadClientMaterial {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uri, err := url.Parse(spiffe)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: spiffe},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		URIs:         []*url.URL{uri},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, c.caCert, &key.PublicKey, c.caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	m := &workloadClientMaterial{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}),
	}
	der, err = x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	m.keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return m
}

// fakeResources is a minimal ResourceManager that returns deterministic results
// so the handler can complete a real Execute round trip.
type fakeResources struct{}

func (fakeResources) Create(_ context.Context, app, env, name string) (objectstorage.RuntimeResourceBinding, error) {
	return objectstorage.RuntimeResourceBinding{
		ResourceID:      "res-" + name,
		Endpoint:        "https://seaweedfs-access:8443",
		Bucket:          "runtime",
		Region:          "local",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	}, nil
}

func (fakeResources) Get(_ context.Context, app, env, name string) (objectstorage.RuntimeResourceBinding, error) {
	return fakeResources{}.Create(context.Background(), app, env, name)
}

func (fakeResources) Delete(_ context.Context, _, _, _ string) error { return nil }

// clientCertPaths writes a client material to the CA temp dir and returns the
// cert/key paths.
func (c *testCA) clientCertPaths(t *testing.T, m *workloadClientMaterial) (certPath, keyPath string) {
	t.Helper()
	certPath = filepath.Join(c.dir, "client-cert.pem")
	keyPath = filepath.Join(c.dir, "client-key.pem")
	c.write(t, "client-cert.pem", m.certPEM, 0o644)
	c.write(t, "client-key.pem", m.keyPEM, 0o600)
	return certPath, keyPath
}

func newMutualTLSServer(t *testing.T, ca *testCA) *httptest.Server {
	t.Helper()
	handler := newExecuteCountingHandler(t)
	mux := http.NewServeMux()
	mux.Handle("GET /readyz", &checkReachedRecorder{})
	mux.Handle("/internal/", handler)
	server := httptest.NewUnstartedServer(mux)
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.caCertPEM) {
		t.Fatal("failed to parse test CA for client trust")
	}
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ca.serverKey})
	cert, err := tls.X509KeyPair(ca.serverCert, serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS.Certificates = []tls.Certificate{cert}
	server.StartTLS()
	return server
}

// newMutualTLSServerWithHandler builds the same handler-backed mutual-TLS
// server as newMutualTLSServer, but also returns the execute-path wrapper and
// the /readyz check-path recorder so a test can assert:
//   - the check route Check() drives was reached (recorder.entered) or, for a
//     refused handshake, refused before dispatch;
//   - the execute route Execute() drives was reached (wrapper.reached).
//
// The two routes have independent reach-flags, so the observation is never
// conflated across paths.
//
// The wrapper is a TEST-ONLY executeCountingHandler around the production
// *Handler: it preserves exact handler behavior while exposing whether the
// execute route was entered, without adding any test instrumentation to the
// production Handler (which must stay byte-identical to develop).
func newMutualTLSServerWithHandler(t *testing.T, ca *testCA) (*httptest.Server, *executeCountingHandler, *checkReachedRecorder) {
	t.Helper()
	handler := newExecuteCountingHandler(t)
	recorder := &checkReachedRecorder{}
	mux := http.NewServeMux()
	mux.Handle("GET /readyz", recorder)
	mux.Handle("/internal/", handler)
	server := httptest.NewUnstartedServer(mux)
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.caCertPEM) {
		t.Fatal("failed to parse test CA for client trust")
	}
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ca.serverKey})
	cert, err := tls.X509KeyPair(ca.serverCert, serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS.Certificates = []tls.Certificate{cert}
	server.StartTLS()
	return server, handler, recorder
}

// TestCheckNegativesWrongRecorderMutationControl is the MUTATION CONTROL for the
// reviewer's counter-test (review-1357e). It proves the negative observation is
// discriminating: it builds a BUGGY server where the /readyz handler
// erroneously dispatches to the /internal/ execute handler (a realistic wrong-
// wiring variant, test-only, never in a production path). On that server the
// WRONG recorder (wrapper.reached, the execute handler wrapper) goes TRUE even
// though the route Check() actually drives — the /readyz checkReachedRecorder —
// is what was really served.
//
// The assertion MUST fire RED here (executeReached==true on the execute handler)
// while the correct observation (the /readyz recorder being entered) proves the
// wrong recorder is observing a route Check() never dispatched to. If this
// control cannot go RED, the negative observation is vacuous and does not
// discriminate the bug — the test would have to be abandoned.
//
// A successful handshake is required (no client-cert refusal), so Check() actually
// reaches the /readyz route; the buggy handler then forwards to the execute
// handler, flipping wrapper.reached to true.
func TestCheckNegativesWrongRecorderMutationControl(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	// Build a BUGGY readyz route: its handler serves /readyz AND, misconfigured,
	// forwards to the execute handler with a synthetic POST request.
	execute := newExecuteCountingHandler(t)
	recorder := &checkReachedRecorder{}
	buggyReadyz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.entered.Store(true)
		// Misconfigured dispatch: forward to the execute route. The execute route
		// is the executeCountingHandler wrapper (the wrong recorder), so routing
		// through it flips execute.reached even though Check() drove /readyz.
		extra := *r
		extra.Method = http.MethodPost
		extra.URL.Path = "/internal/v1/execute"
		extra.Header.Set("Content-Type", "application/json")
		extra.Body = io.NopCloser(strings.NewReader(`{"capability":"object-storage.s3/v1","operation":"runtime.create","name":"asset-1"}`))
		execute.ServeHTTP(w, &extra)
	})
	mux := http.NewServeMux()
	mux.Handle("GET /readyz", buggyReadyz)
	mux.Handle("/internal/", execute)
	server := httptest.NewUnstartedServer(mux)
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.caCertPEM) {
		t.Fatal("failed to parse test CA for client trust")
	}
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ca.serverKey})
	cert, err := tls.X509KeyPair(ca.serverCert, serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS.Certificates = []tls.Certificate{cert}
	server.StartTLS()
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Check() reaches the /readyz route (successful handshake). The buggy handler
	// forwards to the execute handler, so the WRONG recorder observes the request.
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("readyz over mutual-TLS: %v", err)
	}

	// MUTATION CONTROL assertion: on the buggy server the execute handler was
	// reached, proving wrapper.reached is TRUE on a route Check() never
	// actually serviced. This is exactly the false-positive the reviewer warns
	// about: asserting this recorder would (wrongly) claim the execute path.
	if got := execute.reached.Load(); !got {
		t.Fatalf("mutation control failed: execute handler must be reached via buggy readyz dispatch (reached=%v)", got)
	}
	// The correct recorder (the /readyz route Check drives) was entered too.
	if !recorder.entered.Load() {
		t.Fatalf("mutation control failed: /readyz recorder must be entered (entered=%v)", recorder.entered.Load())
	}
}

const workloadSPIFFE = "spiffe://baseharbor/apps/myservice/prod"

// TestCheckNegativesObserveReadyRoute is the reviewer's counter-test (review-1357e):
// it proves that the NEGATIVE Check() assertion observes the EXACT route Check()
// drives — the /readyz checkReachedRecorder — AND the /internal/ execute handler.
// For every negative case (missing client cert, foreign CA) the test asserts BOTH:
//   - readyReached == false  (the /readyz route Check() drives was never entered)
//   - executeReached == false (the /internal/ handler was never entered either)
//
// and the error is a classified TLS/handshake/verification cause.
//
// On the NORMAL server (correct wiring) a refused handshake reaches NEITHER
// route, so both flags stay false and the test is GREEN. This is the
// discriminating green test: the negative is now observed on the correct recorder
// (the /readyz route Check drives), not vacuously on the execute handler alone.
// The mutation control (TestCheckNegativesWrongRecorderMutationControl) proves the
// observation is discriminating: on buggy wiring the wrong recorder goes TRUE.
func TestCheckNegativesObserveReadyRoute(t *testing.T) {
	// --- Negative 1: missing client certificate, Check() drives /readyz ---
	func() {
		ca := newTestCA(t)
		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca.caCertPEM) {
			t.Fatal("failed to parse test CA for client trust")
		}
		tr := &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      roots,
			Certificates: []tls.Certificate{},
		}}
		c := &Client{endpoint: server.URL, http: &http.Client{Transport: tr}}

		err := c.Check(context.Background())
		// The negative MUST be observed on the exact route Check() drives.
		if recorder.entered.Load() {
			t.Fatalf("readyz check-path recorder must not be entered by a client presenting no certificate (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("execute handler must not be reached by a client presenting no certificate (reached=%v)", got)
		}
		assertTLSHandshakeCause(t, err)
	}()

	// --- Negative 2: foreign client CA, Check() drives /readyz ---
	func() {
		ca := newTestCA(t)
		foreignCA := newTestCA(t)
		client := foreignCA.workloadClient(t, workloadSPIFFE)
		certPath, keyPath := foreignCA.clientCertPaths(t, client)

		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		c, err := NewClient(ClientConfig{
			URL:      server.URL,
			CAFile:   ca.caCertPath,
			CertFile: certPath,
			KeyFile:  keyPath,
		})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}

		err = c.Check(context.Background())
		// The negative MUST be observed on the exact route Check() drives.
		if recorder.entered.Load() {
			t.Fatalf("readyz check-path recorder must not be entered by a refused foreign-CA handshake (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("execute handler must not be reached by a refused foreign-CA handshake (reached=%v)", got)
		}
		assertTLSHandshakeCause(t, err)
	}()
}

// TestMutualTLSRejectsForeignClientCA fails when the client presents a

// TestMutualTLSExecutePositive drives a real two-way TLS handshake: the client
// presents a workload identity signed by the test CA, the server verifies the
// client certificate chain AND the SPIFFE identity, and a full Execute round
// trip completes with HTTP 200.
func TestMutualTLSExecutePositive(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	server, execute, _ := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := c.Execute(context.Background(), runtimeoperation.Request{
		Capability:   ObjectStorageS3V1,
		Operation:    "runtime.create",
		ResourceName: "asset-1",
	})
	if err != nil {
		t.Fatalf("Execute over mutual-TLS: %v", err)
	}
	if result.ResourceID != "res-asset-1" {
		t.Fatalf("unexpected resource id: %q", result.ResourceID)
	}
	// Positive counter-flag on the execute path: the /internal/ handler was
	// reached (its own per-path flag flipped true on valid material).
	if !execute.reached.Load() {
		t.Fatalf("execute handler was not reached on valid material")
	}
}

// TestMutualTLSRejectsMissingClientCertificate fails the handshake when the
// client presents no certificate at all: the client is built without its own
// identity, and the server (which requires and verifies client certs) refuses
// the handshake. The refusal happens at the TLS layer before ServeHTTP runs;
// the handler sets an atomic HandlerReached flag on entry, so a refused
// handshake must leave it false, and the error must carry a TLS/handshake/
// verification cause.
func TestMutualTLSRejectsMissingClientCertificate(t *testing.T) {
	ca := newTestCA(t)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	// Client that presents NO certificate: a transport with an empty
	// Certificate slice and no GetClientCertificate callback.
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca.caCertPEM) {
		t.Fatal("failed to parse test CA for client trust")
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      roots,
		Certificates: []tls.Certificate{},
	}}
	c := &Client{endpoint: server.URL, http: &http.Client{Transport: tr}}

	err := c.Check(context.Background())
	// The negative MUST be observed on the EXACT route Check() drives: the
	// /readyz checkReachedRecorder (readyReached==false) AND the execute-path
	// handler (executeReached==false). The old assertion observed only the
	// execute handler — the wrong recorder — and proved nothing about the route
	// Check() actually drove. Both flags must stay false on a refused handshake.
	assertTLSHandshakeCause(t, err)
	// Negative readyReached: on the refused handshake the /readyz route Check()
	// drives was never entered (readyReached==false).
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered by a client presenting no certificate (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a client presenting no certificate (reached=%v)", got)
	}
	// Positive readyReached (counter-flag): on valid material the same /readyz
	// route Check() drives must be entered (readyReached==true).
	{
		ca2 := newTestCA(t)
		client2 := ca2.workloadClient(t, workloadSPIFFE)
		certPath2, keyPath2 := ca2.clientCertPaths(t, client2)
		server2, _, rec2 := newMutualTLSServerWithHandler(t, ca2)
		defer server2.Close()
		c2, cerr := NewClient(ClientConfig{
			URL:      server2.URL,
			CAFile:   ca2.caCertPath,
			CertFile: certPath2,
			KeyFile:  keyPath2,
		})
		if cerr != nil {
			t.Fatalf("NewClient: %v", cerr)
		}
		if cerr := c2.Check(context.Background()); cerr != nil {
			t.Fatalf("readyz over mutual-TLS: %v", cerr)
		}
		if !rec2.entered.Load() {
			t.Fatalf("readyz check-path recorder must be entered on valid material (entered=%v)", rec2.entered.Load())
		}
	}
}

// TestMutualTLSRejectsSelfSignedClientCert fails the handshake when the client
// presents a self-signed identity that is not issued by any CA the server
// trusts. The refusal happens at the TLS layer before ServeHTTP runs; the
// handler must not be reached and the error must carry a TLS/handshake/
// verification cause.
func TestMutualTLSRejectsSelfSignedClientCert(t *testing.T) {
	ca := newTestCA(t)
	// Self-signed client cert NOT issued by the server's test CA.
	selfKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "selfsigned"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &selfKey.PublicKey, selfKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(ca.dir, "selfsigned-cert.pem")
	keyPath := filepath.Join(ca.dir, "selfsigned-key.pem")
	ca.write(t, "selfsigned-cert.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o644)
	selfDER, err := x509.MarshalPKCS8PrivateKey(selfKey)
	if err != nil {
		t.Fatal(err)
	}
	ca.write(t, "selfsigned-key.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: selfDER}), 0o600)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.Check(context.Background())
	assertTLSHandshakeCause(t, err)
	// The negative MUST be observed on the EXACT route Check() drives: the
	// /readyz checkReachedRecorder (readyReached==false) AND the execute-path
	// handler (executeReached==false). Both flags must stay false on a refused
	// handshake — observing only the execute handler was the wrong-recorder bug.
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered by a refused self-signed handshake (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (reached=%v)", got)
	}
}

// TestMutualTLSRejectsForeignClientCA fails when the client presents a
// certificate from a different CA that the server does not trust. The refusal
// happens at the TLS layer; the handler must not be reached and the error must
// carry a TLS/handshake/verification cause.
func TestMutualTLSRejectsForeignClientCA(t *testing.T) {
	ca := newTestCA(t)
	foreignCA := newTestCA(t)
	client := foreignCA.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := foreignCA.clientCertPaths(t, client)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.Check(context.Background())
	// The negative MUST be observed on the EXACT route Check() drives: the
	// /readyz checkReachedRecorder (readyReached==false) AND the execute-path
	// handler (executeReached==false). Both flags must stay false on a refused
	// handshake — observing only the execute handler was the wrong-recorder bug.
	assertTLSHandshakeCause(t, err)
	// Negative readyReached: on the refused foreign-CA handshake the /readyz
	// route Check() drives was never entered (readyReached==false).
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered by a refused foreign-CA handshake (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (reached=%v)", got)
	}
	// Positive readyReached (counter-flag): on valid material the same /readyz
	// route Check() drives must be entered (readyReached==true).
	{
		ca2 := newTestCA(t)
		client2 := ca2.workloadClient(t, workloadSPIFFE)
		certPath2, keyPath2 := ca2.clientCertPaths(t, client2)
		server2, _, rec2 := newMutualTLSServerWithHandler(t, ca2)
		defer server2.Close()
		c2, cerr := NewClient(ClientConfig{
			URL:      server2.URL,
			CAFile:   ca2.caCertPath,
			CertFile: certPath2,
			KeyFile:  keyPath2,
		})
		if cerr != nil {
			t.Fatalf("NewClient: %v", cerr)
		}
		if cerr := c2.Check(context.Background()); cerr != nil {
			t.Fatalf("readyz over mutual-TLS: %v", cerr)
		}
		if !rec2.entered.Load() {
			t.Fatalf("readyz check-path recorder must be entered on valid material (entered=%v)", rec2.entered.Load())
		}
	}
}

// TestMutualTLSRejectsCorruptedClientCertKey fails when the client certificate
// or key material is corrupted (fail-closed): the key cannot be loaded so the
// client is never constructed and the handshake never runs. The error must carry
// a TLS/handshake/verification cause and the handler must never be reached.
//
// NOTE: The single "broken client cert/key" case is now also split into two
// distinct root-cause tests (TestMutualTLSRejectsMalformedClientCertValidKey and
// TestMutualTLSRejectsMalformedClientKeyValidCert) that pin the exact load
// failure. This test documents the original combined failure still holds.
func TestMutualTLSRejectsCorruptedClientCertKey(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)

	// Corrupt the client key bytes on disk: rewrite the PEM body with invalid
	// base64 so the key cannot be parsed (fail-closed at load time).
	certPath := filepath.Join(ca.dir, "client-cert.pem")
	keyPath := filepath.Join(ca.dir, "client-key-corrupted.pem")
	ca.write(t, "client-cert.pem", client.certPEM, 0o644)
	ca.write(t, "client-key-corrupted.pem",
		[]byte("-----BEGIN PRIVATE KEY-----\nMIIinvalidNotValidBase64DataXXXX\n-----END PRIVATE KEY-----\n"), 0o600)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	// Corrupted key => NewClient fails closed before any handshake attempt.
	_, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	// The failure is a client-identity load error (no handshake runs), so the
	// /readyz check path never observes a handshake. The error is non-nil and
	// both reach-flags stay false on the exact route Check() drives AND the
	// execute handler.
	if err == nil {
		t.Fatalf("expected NewClient to fail closed on corrupted client key material")
	}
	// Root cause is a KEY parse failure (valid cert, garbage key).
	if !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected a concrete KEY-verification load cause, got: %q", err.Error())
	}
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered when client identity fails to load (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a client that failed to load (reached=%v)", got)
	}
}

// TestMutualTLSRejectsMalformedClientCertValidKey isolates the CERTIFICATE
// failure: the client's certificate PEM is garbage (fails x509 parsing) while
// the private key is a valid, loadable key. NewClient therefore fails closed at
// the CERTIFICATE parse, and the error must name the certificate as the root
// cause ("x509: malformed certificate"). The client is never constructed, so no
// handshake runs and the execute handler must never be reached.
//
// This is the distinct counterpart to TestMutualTLSRejectsMalformedClientKeyValidCert:
// here the CERTIFICATE is the broken half, not the key.
func TestMutualTLSRejectsMalformedClientCertValidKey(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)

	// Keep the valid key, corrupt only the certificate body so it PEM-decodes
	// but fails x509 parsing.
	corruptCertPath := filepath.Join(ca.dir, "client-cert-malformed.pem")
	corruptKeyPath := filepath.Join(ca.dir, "client-key-valid.pem")
	ca.write(t, "client-cert-malformed.pem",
		[]byte("-----BEGIN CERTIFICATE-----\nMIIinvalidNotValidBase64DataXXXX\n-----END CERTIFICATE-----\n"), 0o644)
	ca.write(t, "client-key-valid.pem", client.keyPEM, 0o600)

	server := newMutualTLSServer(t, ca)
	defer server.Close()

	// NewClient fails at the CERTIFICATE load; the key is valid, so the error is
	// a certificate problem, not a key problem.
	_, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: corruptCertPath,
		KeyFile:  corruptKeyPath,
	})
	if err == nil {
		t.Fatalf("expected NewClient to fail closed on a malformed client certificate")
	}
	if strings.Contains(err.Error(), "private key") {
		t.Fatalf("root cause must be a CERTIFICATE failure, not a key failure, got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("expected a concrete certificate load cause, got: %q", err.Error())
	}
}

// TestMutualTLSRejectsMalformedClientKeyValidCert isolates the PRIVATE KEY
// failure: the client's private key PEM is garbage (fails x509 parsing) while
// the certificate is valid. NewClient therefore fails closed at the KEY parse,
// and the error must name the key as the root cause
// ("tls: failed to parse private key"). The client is never constructed, so no
// handshake runs and the execute handler must never be reached.
//
// This is the distinct counterpart to TestMutualTLSRejectsMalformedClientCertValidKey:
// here the KEY is the broken half, not the certificate.
func TestMutualTLSRejectsMalformedClientKeyValidCert(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)

	// Keep the valid cert, corrupt only the key body so it PEM-decodes but fails
	// x509 parsing.
	corruptCertPath := filepath.Join(ca.dir, "client-cert-valid.pem")
	corruptKeyPath := filepath.Join(ca.dir, "client-key-malformed.pem")
	ca.write(t, "client-cert-valid.pem", client.certPEM, 0o644)
	ca.write(t, "client-key-malformed.pem",
		[]byte("-----BEGIN PRIVATE KEY-----\nMIICdgIBADANBgkq\n-----END PRIVATE KEY-----\n"), 0o600)

	server := newMutualTLSServer(t, ca)
	defer server.Close()

	// NewClient fails at the KEY load; the cert is valid, so the error is a key
	// problem, not a certificate problem.
	_, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: corruptCertPath,
		KeyFile:  corruptKeyPath,
	})
	if err == nil {
		t.Fatalf("expected NewClient to fail closed on a malformed client key")
	}
	if strings.Contains(err.Error(), "certificate") {
		t.Fatalf("root cause must be a KEY failure, not a certificate failure, got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected a concrete private-key load cause, got: %q", err.Error())
	}
}

// TestMutualTLSRejectsInvalidCAPEM fails when the client trust bundle is not a
// valid PEM certificate.
func TestMutualTLSRejectsInvalidCAPEM(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	invalidCA := filepath.Join(ca.dir, "invalid-ca.pem")
	ca.write(t, "invalid-ca.pem", []byte("-----BEGIN CERTIFICATE-----\nNOTVALIDBASE64\n-----END CERTIFICATE-----\n"), 0o644)

	server := newMutualTLSServer(t, ca)
	defer server.Close()

	err := func() error {
		_, e := NewClient(ClientConfig{
			URL:      server.URL,
			CAFile:   invalidCA,
			CertFile: certPath,
			KeyFile:  keyPath,
		})
		return e
	}()
	if err == nil {
		t.Fatalf("expected NewClient to reject an invalid CA PEM")
	}
	// Punkt 2: assert the concrete server-side verification cause. An invalid
	// client trust bundle is rejected with a specific "runtime executor CA is
	// invalid" message at load time — a concrete trust-bundle verification
	// failure, not an unspecified transport abort.
	if got := err.Error(); !strings.Contains(got, "CA is invalid") {
		t.Fatalf("expected a concrete CA-verification cause, got: %q", got)
	}
}

// TestMutualTLSRejectsWrongServerCA fails when the client trusts a different CA
// than the one the server presents (server authentication must also fail
// closed). The refusal happens at the TLS layer; the handler must not be
// reached and the error must carry a TLS/handshake/verification cause.
func TestMutualTLSRejectsWrongServerCA(t *testing.T) {
	ca := newTestCA(t)
	otherCA := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   otherCA.caCertPath, // trusts the wrong CA
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.Check(context.Background())
	assertTLSHandshakeCause(t, err)
	// The negative MUST be observed on the EXACT route Check() drives: the
	// /readyz checkReachedRecorder (readyReached==false) AND the execute-path
	// handler (executeReached==false). Both flags must stay false on a refused
	// handshake — observing only the execute handler was the wrong-recorder bug.
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered by a refused wrong-server-CA handshake (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (reached=%v)", got)
	}
}

// TestMutualTLSRejectsHandshakeDoesNotReachHandler proves that a negative TLS
// handshake never dispatches to the handler body: the server requires and
// verifies client certificates, so an untrusted client certificate is refused
// at the handshake layer before ServeHTTP runs. The handler sets an atomic
// HandlerReached flag on entry; a refused handshake must leave it false, and a
// subsequent Execute over the rejected identity must not reach the handler.
func TestMutualTLSRejectsHandshakeDoesNotReachHandler(t *testing.T) {
	ca := newTestCA(t)
	// Client cert from a foreign CA the server does not trust.
	foreignCA := newTestCA(t)
	client := foreignCA.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := foreignCA.clientCertPaths(t, client)

	// Build the server with direct access to the *Handler under test via a
	// TEST-ONLY executeCountingHandler wrapper (the /readyz route is served by a
	// checkReachedRecorder, the route Check() drives), so this test proves a
	// refused handshake never dispatched to the execute handler.
	execute := newExecuteCountingHandler(t)
	recorder := &checkReachedRecorder{}
	mux := http.NewServeMux()
	mux.Handle("GET /readyz", recorder)
	mux.Handle("/internal/", execute)
	server := httptest.NewUnstartedServer(mux)
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.caCertPEM) {
		t.Fatal("failed to parse test CA for client trust")
	}
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ca.serverKey})
	cert, err := tls.X509KeyPair(ca.serverCert, serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS.Certificates = []tls.Certificate{cert}
	server.StartTLS()
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// First a Check that is refused at the handshake.
	checkErr := c.Check(context.Background())
	if checkErr == nil {
		t.Fatalf("expected handshake failure with an untrusted client certificate")
	}
	assertTLSHandshakeCause(t, checkErr)
	// The negative MUST be observed on the EXACT route Check() drives: the
	// /readyz checkReachedRecorder (readyReached==false) AND the execute-path
	// handler (executeReached==false). Both flags stay false on a refused
	// handshake — observing only the execute handler was the wrong-recorder bug.
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered by a refused foreign-CA handshake (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (reached=%v)", got)
	}

	// Then an Execute against the same rejected identity.
	if _, err := c.Execute(context.Background(), runtimeoperation.Request{
		Capability:   ObjectStorageS3V1,
		Operation:    "runtime.create",
		ResourceName: "asset-1",
	}); err == nil {
		t.Fatalf("expected Execute to fail on the refused handshake")
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a refused Execute (reached=%v)", got)
	}
}

// TestNegativesObserveReadyRouteClientBrokenCert (runde 9, w68-ci-access-i) is the
// "defektes-cert" negative of the 12-row matrix. It drives a REAL NewClient + Check
// against a broken client identity: the certificate PEM is valid but the private key
// body is truncated (PEM-decodes yet fails x509 parsing → NewClient fails closed at
// load time with "tls: failed to parse private key"). The refused identity must reach
// NEITHER the /readyz check-path recorder (ready-entered==false) NOR the execute
// handler (execute-reached==false). The Execute path is then exercised against the
// same refused handshake via a foreign-CA client to prove Execute likewise never
// reaches the handler on a refused handshake.
func TestNegativesObserveReadyRouteClientBrokenCert(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)

	// Intact certificate (written via workloadClient's valid certPEM above), but a
	// truncated private key body that PEM-decodes yet fails x509 parsing → NewClient
	// fails closed at load time with "tls: failed to parse private key".
	certPath := filepath.Join(ca.dir, "client-cert.pem")
	keyPath := filepath.Join(ca.dir, "client-key-broken.pem")
	ca.write(t, "client-cert.pem", client.certPEM, 0o644)
	ca.write(t, "client-key-broken.pem",
		[]byte("-----BEGIN PRIVATE KEY-----\nMIICdgIBADANBgkq\n-----END PRIVATE KEY-----\n"), 0o600)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	// Broken key => NewClient fails closed before any handshake attempt. The
	// /readyz check-path recorder must never be entered (ready-entered==false)
	// AND the execute handler must never be reached (execute-reached==false)
	// because no handshake ever runs.
	_, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err == nil {
		t.Fatalf("expected NewClient to fail closed on a broken (truncated) client key")
	}
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered when broken client identity fails to load (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("execute handler must not be reached when broken client identity fails to load (reached=%v)", got)
	}

	// Drive the Execute path against a genuinely refused handshake to prove Execute
	// also never reaches the handler on a refused identity. Use a foreign CA so the
	// client cert loads fine but the handshake is refused server-side (a real
	// refused-execute, not a load failure): the handler guard must hold.
	foreignCA := newTestCA(t)
	fc := foreignCA.workloadClient(t, workloadSPIFFE)
	fcCert, fcKey := foreignCA.clientCertPaths(t, fc)
	fcClient, cerr := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: fcCert,
		KeyFile:  fcKey,
	})
	if cerr != nil {
		t.Fatalf("expected NewClient to build a foreign-CA client (handshake refused later, not at load): %v", cerr)
	}
	if _, e := fcClient.Execute(context.Background(), runtimeoperation.Request{
		Capability:   ObjectStorageS3V1,
		Operation:    "runtime.create",
		ResourceName: "asset-1",
	}); e == nil {
		t.Fatalf("expected Execute to fail on the refused foreign-CA handshake")
	}
	// After a refused Execute the execute route must still be un-entered.
	if got := execute.reached.Load(); got {
		t.Fatalf("handler must not be reached by a refused-identity Execute (reached=%v)", got)
	}
	// Re-read the check-path AFTER execute (Finding 1): execute must not have
	// caused the /readyz route Check() drives to be entered either.
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered by a refused-identity Execute (entered=%v)", recorder.entered.Load())
	}
}

// TestMutualTLSRejectsInvalidCAFileLoadPhase (runde 9, w68-ci-access-i) proves that an
// invalid CA file on the CLIENT side is rejected at the LOAD phase — BEFORE any request
// (Check/Execute) is issued. The load error must surface before the /readyz check-path
// recorder is ever touched, so there are NO null/recorder accesses on the refused path.
// The error carries the concrete "runtime executor CA is invalid" cause.
func TestMutualTLSRejectsInvalidCAFileLoadPhase(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	// Client-side CA file is garbage (invalid PEM body) → load fails before request.
	invalidCA := filepath.Join(ca.dir, "invalid-ca-file.pem")
	ca.write(t, "invalid-ca-file.pem", []byte("-----BEGIN CERTIFICATE-----\nGARBAGE-INVALID-PEM-BODY\n-----END CERTIFICATE-----\n"), 0o644)

	server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	// The load error must come BEFORE the request. Building the client fails at
	// CA parse; no Check/Execute is issued, so neither recorder nor handler is
	// touched (no null access).
	_, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   invalidCA,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err == nil {
		t.Fatalf("expected NewClient to reject a garbage client CA file at load phase")
	}
	if got := err.Error(); !strings.Contains(got, "CA is invalid") {
		t.Fatalf("expected a concrete CA-verification load cause, got: %q", got)
	}
	// Phase assertion: load-phase failure — no handshake ran, so both reach-flags
	// stay false on the exact route Check() drives AND the execute handler.
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be touched by a client-side CA load failure (entered=%v)", recorder.entered.Load())
	}
	if got := execute.reached.Load(); got {
		t.Fatalf("execute handler must not be reached by a client-side CA load failure (reached=%v)", got)
	}
}

// TestExecuteNeverReachedOnEveryNegativeInput is the 12-row matrix of the
// client-cert access guard (mission w70-ci-access-j, review-1357i). For each of
// the six handshake/identity failure inputs it drives a REAL NewClient + Check
// against the same RequireAndVerifyClientCert server, then drives a REAL Execute
// against a refused identity, asserting for BOTH routes:
//
//   - executeReached == false : the /internal/ execute route was never entered on
//     a refused handshake (proven via the executeCountingHandler wrapper, since
//     production handler.go carries no test counter);
//   - readyReached == false   : the /readyz check route was never entered either;
//
// and re-reads the readyz recorder AFTER the Execute so the two observations are
// never conflated across time. Each matrix row accounts for exactly two route
// combinations — a Check combination (the /readyz route) and an Execute
// combination (the /internal/ route). The twelve combinations are the six
// negative identities x the two routes, i.e. {Check, Execute} x six inputs.
//
// The six failure inputs are:
//  1. no client cert (empty client identity),
//  2. self-signed client cert (not issued by any trusted CA),
//  3. foreign client CA (client cert signed by an untrusted CA),
//  4. malformed client cert (garbage cert body, valid key) — LOAD PHASE,
//  5. wrong server CA (client trusts a CA the server did not present),
//  6. malformed client CA (load phase: garbage client CA file).
//
// Rows 1, 2, 3 and 5 build a client and drive a REAL Check AND a REAL Execute on
// two independent routes (eight real driver calls). Rows 4 and 6 fail closed at
// the client-load phase before any request is issued, so both the Check and the
// Execute are a documented NULL-CALL on that row — neither route is driven. The
// reach-flags must still be observed false on the exact routes, proving the
// load-phase failure is observed on the correct recorder rather than vacuously.
//
// Each row is a SEPARATE closure with its own fresh test CA + server so a
// client-load failure is never mistaken for a handshake refusal.
//
// The miswire counter-control (TestExecuteMisrouteGoesRed) at the end of this
// file proves the twelve-route observation is discriminating: on swapped
// routing the matching assertions go RED.
func TestExecuteNeverReachedOnEveryNegativeInput(t *testing.T) {
	var combinedPaths int // N==M counter: every row contributes Check+Execute == 2 combinations

	// 1. No client cert: a transport with an empty certificate slice presents no
	// identity, so the server refuses the handshake before dispatch.
	func() {
		ca := newTestCA(t)
		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca.caCertPEM) {
			t.Fatal("failed to parse test CA for client trust")
		}
		tr := &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			RootCAs:      roots,
			Certificates: []tls.Certificate{},
		}}
		c := &Client{endpoint: server.URL, http: &http.Client{Transport: tr}}

		// REAL Check on the /readyz route.
		checkErr := c.Check(context.Background())
		assertTLSHandshakeCause(t, checkErr)
		if recorder.entered.Load() {
			t.Fatalf("row 1: readyz check-path recorder must not be entered without a client cert (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 1: execute handler must not be reached without a client cert (reached=%v)", got)
		}

		// REAL Execute on the /internal/ route against the same refused identity.
		if _, e := c.Execute(context.Background(), runtimeoperation.Request{
			Capability:   ObjectStorageS3V1,
			Operation:    "runtime.create",
			ResourceName: "asset-1",
		}); e == nil {
			t.Fatalf("row 1: Execute must fail on a refused handshake without a client cert")
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 1: execute handler must not be reached by a refused Execute (reached=%v)", got)
		}
		combinedPaths += 2 // Check + Execute, both real
		// Re-read readyz AFTER execute (row 1): execute must not have entered the
		// check route either.
		if recorder.entered.Load() {
			t.Fatalf("row 1: readyz check-path recorder must not be entered after a refused execute (entered=%v)", recorder.entered.Load())
		}
	}()

	// 2. Self-signed client cert: issued by nobody the server trusts.
	func() {
		ca := newTestCA(t)
		selfKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: "selfsigned"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, &template, &template, &selfKey.PublicKey, selfKey)
		if err != nil {
			t.Fatal(err)
		}
		certPath := filepath.Join(ca.dir, "selfsigned-cert.pem")
		keyPath := filepath.Join(ca.dir, "selfsigned-key.pem")
		ca.write(t, "selfsigned-cert.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
		selfDER, err := x509.MarshalPKCS8PrivateKey(selfKey)
		if err != nil {
			t.Fatal(err)
		}
		ca.write(t, "selfsigned-key.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: selfDER}), 0o600)

		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		c, err := NewClient(ClientConfig{
			URL:      server.URL,
			CAFile:   ca.caCertPath,
			CertFile: certPath,
			KeyFile:  keyPath,
		})
		if err != nil {
			t.Fatalf("row 2: NewClient: %v", err)
		}
		// REAL Check on the /readyz route.
		checkErr := c.Check(context.Background())
		assertTLSHandshakeCause(t, checkErr)
		if recorder.entered.Load() {
			t.Fatalf("row 2: readyz check-path recorder must not be entered by a self-signed cert (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 2: execute handler must not be reached by a self-signed cert (reached=%v)", got)
		}
		// REAL Execute on the /internal/ route against the same refused identity.
		if _, e := c.Execute(context.Background(), runtimeoperation.Request{
			Capability:   ObjectStorageS3V1,
			Operation:    "runtime.create",
			ResourceName: "asset-1",
		}); e == nil {
			t.Fatalf("row 2: Execute must fail on a refused handshake with a self-signed cert")
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 2: execute handler must not be reached by a refused Execute (reached=%v)", got)
		}
		combinedPaths += 2 // Check + Execute, both real
		if recorder.entered.Load() {
			t.Fatalf("row 2: readyz check-path recorder must not be entered after a refused execute (entered=%v)", recorder.entered.Load())
		}
	}()

	// 3. Foreign client CA: client cert signed by a CA the server does not trust.
	func() {
		ca := newTestCA(t)
		foreignCA := newTestCA(t)
		client := foreignCA.workloadClient(t, workloadSPIFFE)
		certPath, keyPath := foreignCA.clientCertPaths(t, client)

		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		c, err := NewClient(ClientConfig{
			URL:      server.URL,
			CAFile:   ca.caCertPath,
			CertFile: certPath,
			KeyFile:  keyPath,
		})
		if err != nil {
			t.Fatalf("row 3: NewClient: %v", err)
		}
		// REAL Check on the /readyz route.
		checkErr := c.Check(context.Background())
		assertTLSHandshakeCause(t, checkErr)
		if recorder.entered.Load() {
			t.Fatalf("row 3: readyz check-path recorder must not be entered by a foreign-CA cert (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 3: execute handler must not be reached by a foreign-CA cert (reached=%v)", got)
		}
		// REAL Execute on the /internal/ route against the same refused identity.
		if _, e := c.Execute(context.Background(), runtimeoperation.Request{
			Capability:   ObjectStorageS3V1,
			Operation:    "runtime.create",
			ResourceName: "asset-1",
		}); e == nil {
			t.Fatalf("row 3: Execute must fail on a refused handshake with a foreign-CA cert")
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 3: execute handler must not be reached by a refused Execute (reached=%v)", got)
		}
		combinedPaths += 2 // Check + Execute, both real
		if recorder.entered.Load() {
			t.Fatalf("row 3: readyz check-path recorder must not be entered after a refused execute (entered=%v)", recorder.entered.Load())
		}
	}()

	// 4. Malformed client cert: garbage certificate body, valid key. NewClient
	// fails at the certificate load (a client-load failure, not a handshake).
	//
	// LOAD PHASE null-call: the client never builds, so NEITHER the Check NOR the
	// Execute is issued on this row — both route combinations are documented
	// NULL-CALLS. Both reach-flags must still be observed false on the exact
	// routes, proving the load-phase failure is observed on the correct recorder.
	func() {
		ca := newTestCA(t)
		client := ca.workloadClient(t, workloadSPIFFE)

		corruptCertPath := filepath.Join(ca.dir, "client-cert-malformed-2.pem")
		ca.write(t, "client-cert-malformed-2.pem",
			[]byte("-----BEGIN CERTIFICATE-----\nMIIinvalidNotValidBase64DataXXXX\n-----END CERTIFICATE-----\n"), 0o644)

		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		_, keyPath := ca.clientCertPaths(t, client)
		loadErr := mustLoadErr(t, func() error {
			_, e := NewClient(ClientConfig{
				URL:      server.URL,
				CAFile:   ca.caCertPath,
				CertFile: corruptCertPath,
				KeyFile:  keyPath,
			})
			return e
		})
		if !strings.Contains(loadErr.Error(), "certificate") {
			t.Fatalf("row 4: malformed client cert must fail as a certificate load error, got: %q", loadErr.Error())
		}
		// Load-phase null-call: neither route was driven; observe both false.
		if recorder.entered.Load() {
			t.Fatalf("row 4: readyz check-path recorder must not be touched by a client-side cert load failure (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 4: execute handler must not be reached by a client-side cert load failure (reached=%v)", got)
		}
		combinedPaths += 2 // Check + Execute both documented as NULL-CALL (load phase)
	}()

	// 5. Wrong server CA: client trusts a different CA than the server presents,
	// so server authentication fails at the handshake.
	func() {
		ca := newTestCA(t)
		otherCA := newTestCA(t)
		client := ca.workloadClient(t, workloadSPIFFE)
		certPath, keyPath := ca.clientCertPaths(t, client)

		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		c, err := NewClient(ClientConfig{
			URL:      server.URL,
			CAFile:   otherCA.caCertPath,
			CertFile: certPath,
			KeyFile:  keyPath,
		})
		if err != nil {
			t.Fatalf("row 5: NewClient: %v", err)
		}
		// REAL Check on the /readyz route.
		checkErr := c.Check(context.Background())
		assertTLSHandshakeCause(t, checkErr)
		if recorder.entered.Load() {
			t.Fatalf("row 5: readyz check-path recorder must not be entered by the wrong server CA (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 5: execute handler must not be reached by the wrong server CA (reached=%v)", got)
		}
		// REAL Execute on the /internal/ route against the same refused identity.
		if _, e := c.Execute(context.Background(), runtimeoperation.Request{
			Capability:   ObjectStorageS3V1,
			Operation:    "runtime.create",
			ResourceName: "asset-1",
		}); e == nil {
			t.Fatalf("row 5: Execute must fail on a refused handshake with the wrong server CA")
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 5: execute handler must not be reached by a refused Execute (reached=%v)", got)
		}
		combinedPaths += 2 // Check + Execute, both real
		if recorder.entered.Load() {
			t.Fatalf("row 5: readyz check-path recorder must not be entered after a refused execute (entered=%v)", recorder.entered.Load())
		}
	}()

	// 6. Malformed client CA (load phase): garbage client CA file, rejected
	// before any request is issued.
	//
	// LOAD PHASE null-call: the client never builds, so NEITHER the Check NOR the
	// Execute is issued on this row — both route combinations are documented
	// NULL-CALLS. Both reach-flags must still be observed false on the exact
	// routes, proving the load-phase failure is observed on the correct recorder.
	func() {
		ca := newTestCA(t)
		client := ca.workloadClient(t, workloadSPIFFE)
		certPath, keyPath := ca.clientCertPaths(t, client)

		invalidCA := filepath.Join(ca.dir, "invalid-ca-file-2.pem")
		ca.write(t, "invalid-ca-file-2.pem",
			[]byte("-----BEGIN CERTIFICATE-----\nGARBAGE-INVALID-PEM-BODY\n-----END CERTIFICATE-----\n"), 0o644)

		server, execute, recorder := newMutualTLSServerWithHandler(t, ca)
		defer server.Close()

		loadErr := mustLoadErr(t, func() error {
			_, e := NewClient(ClientConfig{
				URL:      server.URL,
				CAFile:   invalidCA,
				CertFile: certPath,
				KeyFile:  keyPath,
			})
			return e
		})
		if !strings.Contains(loadErr.Error(), "CA is invalid") {
			t.Fatalf("row 6: malformed client CA must fail as a CA load error, got: %q", loadErr.Error())
		}
		if recorder.entered.Load() {
			t.Fatalf("row 6: readyz check-path recorder must not be touched by a client-side CA load failure (entered=%v)", recorder.entered.Load())
		}
		if got := execute.reached.Load(); got {
			t.Fatalf("row 6: execute handler must not be reached by a client-side CA load failure (reached=%v)", got)
		}
		// Load-phase null-call: neither route was driven; observe both false.
		combinedPaths += 2 // Check + Execute both documented as NULL-CALL (load phase)
	}()

	// N==M assertion: every one of the six negative identities contributes exactly
	// two route combinations (one Check /readyz combination + one Execute
	// /internal/ combination) => twelve real or documented-null route
	// combinations. A silent regression that drops a Check or Execute path (the
	// review-1357j miswire) would leave combinedPaths below twelve and fail here.
	if combinedPaths != 12 {
		t.Fatalf("matrix N==M counter mismatch: expected 12 route combinations (Check+Execute x six inputs), got %d", combinedPaths)
	}
}

// mustLoadErr runs a client-construction closure and returns its error, failing
// the test if the client unexpectedly built successfully. It exists so the
// client-load rows (4 and 6) can assert on the concrete load error without
// duplicating the build call across the error and the reach assertions.
func mustLoadErr(t *testing.T, build func() error) error {
	t.Helper()
	err := build()
	if err == nil {
		t.Fatal("expected NewClient to fail closed on the malformed client identity")
	}
	return err
}

// TestExecuteMisrouteGoesRed is the DISCRIMINATION control for the twelve-route
// matrix (TestExecuteNeverReachedOnEveryNegativeInput, review-1357j). It proves
// that the Execute-path observation (executeCountingHandler.reached, the route
// Execute() drives — the /internal/ handler) is discriminating, in both
// directions:
//
//  1. CORRECT wiring: a successful Execute() is served by the execute handler,
//     so execute.reached is TRUE (GREEN). The same assertion is what the review
//     requires the matrix to drive for real.
//  2. SWAPPED wiring (/readyz<->/internal/): a successful Execute() is served by
//     the checkReachedRecorder instead, so execute.reached stays FALSE even
//     though Execute() completes with HTTP 200. The GREEN assertion from step 1
//     therefore goes RED here.
//
// This is the reviewer's miswire counter-control: the identical assertion that
// is GREEN on correct wiring goes RED under a /readyz<->/internal/ swap, which
// is exactly the "tests don't really test the route" failure the review requires
// the matrix to catch. A valid workload identity is required (successful
// mutual-TLS handshake), so Execute() actually dispatches to /internal/v1/execute;
// the swap is what makes the wrong recorder serve it.
func TestExecuteMisrouteGoesRed(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	// buildServer returns the execute-path wrapper and the check recorder. When
	// swap is true the two handlers are wired to the wrong route.
	buildServer := func(swap bool) (*httptest.Server, *executeCountingHandler, *checkReachedRecorder) {
		execute := newExecuteCountingHandler(t)
		recorder := &checkReachedRecorder{}
		mux := http.NewServeMux()
		if swap {
			mux.Handle("GET /readyz", execute) // swapped: check route -> execute handler
			mux.Handle("/internal/", recorder) // swapped: execute route -> check recorder
		} else {
			mux.Handle("GET /readyz", recorder) // correct: check route -> check recorder
			mux.Handle("/internal/", execute)   // correct: execute route -> execute handler
		}
		server := httptest.NewUnstartedServer(mux)
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(ca.caCertPEM) {
			t.Fatal("failed to parse test CA for client trust")
		}
		server.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs:  caPool,
		}
		serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ca.serverKey})
		cert, err := tls.X509KeyPair(ca.serverCert, serverKeyPEM)
		if err != nil {
			t.Fatal(err)
		}
		server.TLS.Certificates = []tls.Certificate{cert}
		server.StartTLS()
		return server, execute, recorder
	}

	// --- GREEN (correct wiring): Execute() is served by the execute handler, so
	// its per-path reach flag must be TRUE.
	server, execute, _ := buildServer(false)
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Execute(context.Background(), runtimeoperation.Request{
		Capability:   ObjectStorageS3V1,
		Operation:    "runtime.create",
		ResourceName: "asset-1",
	}); err != nil {
		t.Fatalf("Execute over mutual-TLS: %v", err)
	}
	if got := execute.reached.Load(); !got {
		t.Fatalf("miswire control failed: execute handler must be reached on correct wiring (reached=%v)", got)
	}

	// --- RED (swapped wiring): the same Execute() is served by the wrong recorder,
	// so execute.reached stays FALSE and the GREEN assertion above fires RED,
	// proving the matrix's Execute-route observation is discriminating and not
	// vacuously green.
	server2, execute2, _ := buildServer(true)
	defer server2.Close()
	c2, err := NewClient(ClientConfig{
		URL:      server2.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient (swap): %v", err)
	}
	// Execute() returns without error (the check recorder answers 200) but the
	// execute handler was never entered.
	if _, err := c2.Execute(context.Background(), runtimeoperation.Request{
		Capability:   ObjectStorageS3V1,
		Operation:    "runtime.create",
		ResourceName: "asset-1",
	}); err != nil {
		t.Fatalf("Execute over swapped mutual-TLS: %v", err)
	}
	if got := execute2.reached.Load(); got {
		t.Fatalf("miswire control failed: execute handler must NOT be reached under /readyz<->/internal/ swap (reached=%v)", got)
	}
}

// mutual-TLS handshake with the correct client identity.
func TestMutualTLSPositiveRoundTripReadyz(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	server, _, recorder := newMutualTLSServerWithHandler(t, ca)
	defer server.Close()

	c, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("readyz over mutual-TLS: %v", err)
	}
	// Positive counter-flag on the check path: the /readyz handler Check drives
	// was reached (its own per-path flag flipped true) on valid material.
	if !recorder.entered.Load() {
		t.Fatalf("check-path /readyz handler was not reached on valid material")
	}
}

var _ = bytes.MinRead

func readClientPair(t *testing.T, dir, certPath, keyPath string) (tls.Certificate, error) {
	t.Helper()
	return tls.LoadX509KeyPair(certPath, keyPath)
}

// Helpers restored verbatim from 195886cc to support the regression tests below.

func newCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "baseharbor-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return ca, key
}

func testTLSClient(t *testing.T) *Client {
	t.Helper()
	ca, caKey := newCA(t)
	return testTLSClientWithCA(t, ca, caKey)
}

func testTLSClientWithCA(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey) *Client {
	t.Helper()
	dir := t.TempDir()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caFile, certFile, keyFile := writeCertFiles(t, dir, ca, caKey, leafKey)

	client, err := NewClient(ClientConfig{
		URL:      "https://127.0.0.1:8443",
		CAFile:   caFile,
		CertFile: certFile,
		KeyFile:  keyFile,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func writeCertFiles(t *testing.T, dir string, ca *x509.Certificate, caKey *ecdsa.PrivateKey, leafKey *ecdsa.PrivateKey) (caFile, certFile, keyFile string) {
	t.Helper()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	caFile = filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	certFile, keyFile = writeLeaf(t, dir, ca, caKey, leafKey, nil, []net.IP{net.ParseIP("127.0.0.1")})
	return caFile, certFile, keyFile
}

func writeLeaf(t *testing.T, dir string, ca *x509.Certificate, caKey *ecdsa.PrivateKey, leafKey *ecdsa.PrivateKey, dnsNames []string, ipAddrs []net.IP) (certFile, keyFile string) {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "baseharbor-client"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	if len(dnsNames) > 0 {
		template.DNSNames = dnsNames
	}
	if len(ipAddrs) > 0 {
		template.IPAddresses = ipAddrs
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	certFile = filepath.Join(dir, "client-cert.pem")
	if err := os.WriteFile(certFile, leafPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	keyFile = filepath.Join(dir, "client-key.pem")
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func caSignedTLSServer(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "baseharbor-server"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(serverCert)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{serverCert}, Leaf: leaf, PrivateKey: serverKey}
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequestClientCert,
	}
	srv.StartTLS()
	return srv
}

// --- Restored regression tests (deleted in 195886cc..067274a1, added verbatim) ---

func TestNewClientRequiresHTTPS(t *testing.T) {
	dir := t.TempDir()
	ca, caKey := newCA(t)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caFile, certFile, keyFile := writeCertFiles(t, dir, ca, caKey, leafKey)

	_, err = NewClient(ClientConfig{URL: "http://127.0.0.1:8443", CAFile: caFile, CertFile: certFile, KeyFile: keyFile})
	if err == nil {
		t.Fatal("expected failure for non-HTTPS URL")
	}
}

func TestNewClientRejectsEmptyURL(t *testing.T) {
	if _, err := NewClient(ClientConfig{}); err == nil {
		t.Fatal("expected failure for empty URL")
	}
}

func TestNewClientRejectsUnparseableURL(t *testing.T) {
	if _, err := NewClient(ClientConfig{URL: ":///"}); err == nil {
		t.Fatal("expected failure for unparseable URL")
	}
}

func TestNewClientRejectsMissingCA(t *testing.T) {
	dir := t.TempDir()
	ca, caKey := newCA(t)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, certFile, keyFile := writeCertFiles(t, dir, ca, caKey, leafKey)
	if _, err := NewClient(ClientConfig{URL: "https://127.0.0.1:8443", CAFile: filepath.Join(dir, "missing.pem"), CertFile: certFile, KeyFile: keyFile}); err == nil {
		t.Fatal("expected failure for missing CA file")
	}
}

func TestCheckReadinessRejectsNon200(t *testing.T) {
	ca, caKey := newCA(t)
	client := testTLSClientWithCA(t, ca, caKey)
	srv := caSignedTLSServer(t, ca, caKey, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	client.endpoint = srv.URL

	if err := client.Check(context.Background()); err == nil {
		t.Fatal("expected readiness failure on non-200")
	}
}

func TestExecuteRoundTripRequiresClientIdentity(t *testing.T) {
	ca, caKey := newCA(t)
	client := testTLSClientWithCA(t, ca, caKey)
	gotIdentity := false
	srv := caSignedTLSServer(t, ca, caKey, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		gotIdentity = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"resource_id":"op-123"}`))
	}))
	defer srv.Close()
	client.endpoint = srv.URL

	result, err := client.Execute(context.Background(), runtimeoperation.Request{
		Capability:   "execute",
		Operation:    "run",
		ResourceName: "node-a",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.ResourceID != "op-123" {
		t.Fatalf("resource_id = %q", result.ResourceID)
	}
	if !gotIdentity {
		t.Fatal("server did not observe client identity (mutual TLS not enforced)")
	}
}

func TestExecuteRejectsNon200(t *testing.T) {
	ca, caKey := newCA(t)
	client := testTLSClientWithCA(t, ca, caKey)
	srv := caSignedTLSServer(t, ca, caKey, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	client.endpoint = srv.URL

	if _, err := client.Execute(context.Background(), runtimeoperation.Request{Capability: "execute", Operation: "run", ResourceName: "node-a"}); err == nil {
		t.Fatal("expected Execute failure on non-200")
	}
}
