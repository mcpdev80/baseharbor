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
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
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

const workloadSPIFFE = "spiffe://baseharbor/apps/myservice/prod"

// TestMutualTLSExecutePositive drives a real two-way TLS handshake: the client
// presents a workload identity signed by the test CA, the server verifies the
// client certificate chain AND the SPIFFE identity, and a full Execute round
// trip completes with HTTP 200.
func TestMutualTLSExecutePositive(t *testing.T) {
	ca := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	server := newMutualTLSServer(t, ca)
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
}

// TestMutualTLSRejectsMissingClientCertificate fails the handshake when the
// client presents a certificate that is not signed by any CA the server trusts
// (the server requires and verifies client certs, so an untrusted/self-signed
// client identity is rejected at the TLS layer).
func TestMutualTLSRejectsMissingClientCertificate(t *testing.T) {
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

	server := newMutualTLSServer(t, ca)
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

	if err := c.Check(context.Background()); err == nil {
		t.Fatalf("expected handshake failure with an untrusted self-signed client certificate")
	}
}

// TestMutualTLSRejectsForeignClientCA fails when the client presents a
// certificate from a different CA that the server does not trust.
func TestMutualTLSRejectsForeignClientCA(t *testing.T) {
	ca := newTestCA(t)
	foreignCA := newTestCA(t)
	client := foreignCA.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := foreignCA.clientCertPaths(t, client)

	server := newMutualTLSServer(t, ca)
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

	if err := c.Check(context.Background()); err == nil {
		t.Fatalf("expected handshake failure with a client certificate from an untrusted CA")
	}
}

// TestMutualTLSRejectsCorruptedClientCertKey fails when the client certificate
// or key material is corrupted (fail-closed): the handshake must not succeed.
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

	server := newMutualTLSServer(t, ca)
	defer server.Close()

	// Corrupted key => NewClient fails closed before any handshake attempt.
	_, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   ca.caCertPath,
		CertFile: certPath,
		KeyFile:  keyPath,
	})
	if err == nil {
		t.Fatalf("expected NewClient to reject corrupted client key material")
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

	if _, err := NewClient(ClientConfig{
		URL:      server.URL,
		CAFile:   invalidCA,
		CertFile: certPath,
		KeyFile:  keyPath,
	}); err == nil {
		t.Fatalf("expected NewClient to reject an invalid CA PEM")
	}
}

// TestMutualTLSRejectsWrongServerCA fails when the client trusts a different CA
// than the one the server presents (server authentication must also fail
// closed).
func TestMutualTLSRejectsWrongServerCA(t *testing.T) {
	ca := newTestCA(t)
	otherCA := newTestCA(t)
	client := ca.workloadClient(t, workloadSPIFFE)
	certPath, keyPath := ca.clientCertPaths(t, client)

	server := newMutualTLSServer(t, ca)
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

	if err := c.Check(context.Background()); err == nil {
		t.Fatalf("expected client to reject a server whose chain does not chain to its trusted CA")
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

	// Build the server with direct access to the *Handler under test.
	handler, err := NewHandler(fakeResources{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
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
	if err := c.Check(context.Background()); err == nil {
		t.Fatalf("expected handshake failure with an untrusted client certificate")
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

	server := newMutualTLSServer(t, ca)
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
