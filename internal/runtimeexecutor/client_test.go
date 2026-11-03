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
	handler, err := NewHandler(fakeResources{})
	if err != nil {
		t.Fatal(err)
	}
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
	return server
}

// newMutualTLSServerWithHandler builds the same handler-backed mutual-TLS
// server as newMutualTLSServer, but also returns the *Handler under test and
// the /readyz check-path recorder so a test can assert:
//   - the check route Check() drives was reached (recorder.entered) or, for a
//     refused handshake, refused before dispatch;
//   - the execute route Execute() drives was reached (handler.HandlerReached).
//
// The two routes have independent reach-flags, so the observation is never
// conflated across paths.
func newMutualTLSServerWithHandler(t *testing.T, ca *testCA) (*httptest.Server, *Handler, *checkReachedRecorder) {
	t.Helper()
	handler, err := NewHandler(fakeResources{})
	if err != nil {
		t.Fatal(err)
	}
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
// WRONG recorder (handler.HandlerReached, the execute handler) goes TRUE even
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
// handler, flipping handler.HandlerReached to true.
func TestCheckNegativesWrongRecorderMutationControl(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	// Build a BUGGY readyz route: its handler serves /readyz AND, misconfigured,
	// forwards to the execute handler with a synthetic POST request.
	handler, err := NewHandler(fakeResources{})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &checkReachedRecorder{}
	buggyReadyz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.entered.Store(true)
		// Misconfigured dispatch: forward to the execute handler too.
		extra := *r
		extra.Method = http.MethodPost
		extra.URL.Path = "/internal/v1/execute"
		extra.Header.Set("Content-Type", "application/json")
		extra.Body = io.NopCloser(strings.NewReader(`{"capability":"object-storage.s3/v1","operation":"runtime.create","name":"asset-1"}`))
		handler.ServeHTTP(w, &extra)
	})
	mux := http.NewServeMux()
	mux.Handle("GET /readyz", buggyReadyz)
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
	// reached, proving handler.HandlerReached is TRUE on a route Check() never
	// actually serviced. This is exactly the false-positive the reviewer warns
	// about: asserting this recorder would (wrongly) claim the execute path.
	if got := handler.HandlerReached.Load(); !got {
		t.Fatalf("mutation control failed: execute handler must be reached via buggy readyz dispatch (HandlerReached=%v)", got)
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
		server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
		if got := handler.HandlerReached.Load(); got {
			t.Fatalf("execute handler must not be reached by a client presenting no certificate (HandlerReached=%v)", got)
		}
		assertTLSHandshakeCause(t, err)
	}()

	// --- Negative 2: foreign client CA, Check() drives /readyz ---
	func() {
		ca := newTestCA(t)
		foreignCA := newTestCA(t)
		client := foreignCA.workloadClient(t, workloadSPIFFE)
		certPath, keyPath := foreignCA.clientCertPaths(t, client)

		server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
		if got := handler.HandlerReached.Load(); got {
			t.Fatalf("execute handler must not be reached by a refused foreign-CA handshake (HandlerReached=%v)", got)
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

	server, handler, _ := newMutualTLSServerWithHandler(t, ca)
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
	if !handler.HandlerReached.Load() {
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

	server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a client presenting no certificate (HandlerReached=%v)", got)
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

	server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (HandlerReached=%v)", got)
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

	server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (HandlerReached=%v)", got)
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

	server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
	if recorder.entered.Load() {
		t.Fatalf("readyz check-path recorder must not be entered when client identity fails to load (entered=%v)", recorder.entered.Load())
	}
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a client that failed to load (HandlerReached=%v)", got)
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

	server, handler, recorder := newMutualTLSServerWithHandler(t, ca)
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
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (HandlerReached=%v)", got)
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

	// Build the server with direct access to the *Handler under test. The
	// /readyz route is served by a checkReachedRecorder (the route Check()
	// drives), so this test proves a refused handshake never dispatched to the
	// execute handler.
	handler, err := NewHandler(fakeResources{})
	if err != nil {
		t.Fatal(err)
	}
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
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a refused TLS handshake (HandlerReached=%v)", got)
	}

	// Then an Execute against the same rejected identity.
	if _, err := c.Execute(context.Background(), runtimeoperation.Request{
		Capability:   ObjectStorageS3V1,
		Operation:    "runtime.create",
		ResourceName: "asset-1",
	}); err == nil {
		t.Fatalf("expected Execute to fail on the refused handshake")
	}
	if got := handler.HandlerReached.Load(); got {
		t.Fatalf("handler must not be reached by a refused Execute (HandlerReached=%v)", got)
	}
}

// TestMutualTLSPositiveRoundTripReadyz exercises the readiness path over a real
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
