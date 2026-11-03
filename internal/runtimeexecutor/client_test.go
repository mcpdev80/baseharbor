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
	dir          string
	caCertPEM    []byte
	caKeyPEM     []byte
	caCert       *x509.Certificate
	caKey        *ecdsa.PrivateKey
	serverCert   []byte
	serverKey    []byte
	caCertPath   string
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
		SerialNumber:       big.NewInt(1),
		Subject:            pkix.Name{CommonName: "baseharbor-runtime-test-ca"},
		NotBefore:          time.Now().Add(-time.Hour),
		NotAfter:           time.Now().Add(24 * time.Hour),
		IsCA:               true,
		BasicConstraintsValid: true,
		KeyUsage:           x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
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
		ResourceID: "res-" + name,
		Endpoint:   "https://seaweedfs-access:8443",
		Bucket:     "runtime",
		Region:     "local",
		AccessKeyID: "ak",
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
