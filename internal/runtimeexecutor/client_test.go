package runtimeexecutor

import (
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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

// writeCertFiles persists PEM-encoded CA and leaf/client certificate/key signed
// by that CA into dir and returns their paths so a Client can be built with
// mutual TLS. caKey signs the produced identities.
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

// caSignedTLSServer starts an httptest TLS server presenting a leaf certificate
// signed by the same test CA as testTLSClient, so the mutual-TLS client verifies
// the server without a custom RootCAs override. Returns the server and its CA
// cert bytes for the handler assertions.
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
