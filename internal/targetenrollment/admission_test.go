package targetenrollment

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type admittedCertificate struct {
	scope   Scope
	serial  string
	expires time.Time
	denied  bool
	calls   int
}

func (r *admittedCertificate) AdmitCertificate(_ context.Context, scope Scope, serial string, expires time.Time) error {
	r.calls++
	if r.denied || scope != r.scope || serial != r.serial || !expires.Equal(r.expires) {
		return ErrDenied
	}
	return nil
}

func authenticatedNodeState(t *testing.T) (tls.ConnectionState, *admittedCertificate) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	issuer := serviceissuer.New(t)
	uri, err := url.Parse(testScope().Identity())
	if err != nil {
		t.Fatal(err)
	}
	clientMaterial, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: testScope().Identity(), URIs: []*url.URL{uri}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	serverMaterial, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: "core.example", DNSNames: []string{"core.example"}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	clientPair, err := tls.X509KeyPair(clientMaterial.Certificate, clientMaterial.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPair, err := tls.X509KeyPair(serverMaterial.Certificate, serverMaterial.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trust.PEM) {
		t.Fatal("test authority trust unavailable")
	}
	clientTransport, serverTransport := net.Pipe()
	defer clientTransport.Close()
	defer serverTransport.Close()
	server := tls.Server(serverTransport, &tls.Config{MinVersion: tls.VersionTLS13, SessionTicketsDisabled: true, Certificates: []tls.Certificate{serverPair},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots})
	client := tls.Client(clientTransport, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "core.example",
		Certificates: []tls.Certificate{clientPair}})
	serverResult := make(chan error, 1)
	go func() { serverResult <- server.HandshakeContext(ctx) }()
	if err := client.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(clientMaterial.Certificate)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := NormalizeCertificateSerial(clientMaterial.Serial)
	if err != nil {
		t.Fatal(err)
	}
	return server.ConnectionState(), &admittedCertificate{scope: testScope(), serial: serial, expires: leaf.NotAfter}
}

func TestNodeAdmissionRequiresTLSAndTheExactActiveRegistryCertificate(t *testing.T) {
	state, registry := authenticatedNodeState(t)
	if err := AdmitTLSNode(context.Background(), state, testScope(), registry); err != nil || registry.calls != 1 {
		t.Fatal("verified TLS 1.3 client did not reach exact registry admission", err)
	}
	registry.denied = true
	if !errors.Is(AdmitTLSNode(context.Background(), state, testScope(), registry), ErrDenied) {
		t.Fatal("CA-trusted but inactive certificate was admitted")
	}
	registry.denied = false
	registry.serial = "different"
	if !errors.Is(AdmitTLSNode(context.Background(), state, testScope(), registry), ErrDenied) {
		t.Fatal("unregistered certificate was admitted")
	}
}

func TestNodeAdmissionRejectsTLSAndScopeDriftBeforeRegistryAccess(t *testing.T) {
	state, registry := authenticatedNodeState(t)
	for _, variant := range []string{"tls12", "incomplete", "anonymous", "nil-leaf", "unverified", "foreign-chain", "foreign-target", "foreign-tenant", "server-leaf", "extra-uri", "expired", "extra-dns"} {
		t.Run(variant, func(t *testing.T) {
			candidate := state
			leaf := *state.PeerCertificates[0]
			candidate.PeerCertificates = []*x509.Certificate{&leaf}
			scope := testScope()
			switch variant {
			case "tls12":
				candidate.Version = tls.VersionTLS12
			case "incomplete":
				candidate.HandshakeComplete = false
			case "anonymous":
				candidate.PeerCertificates = nil
			case "nil-leaf":
				candidate.PeerCertificates = []*x509.Certificate{nil}
			case "unverified":
				candidate.VerifiedChains = nil
			case "foreign-chain":
				candidate.VerifiedChains = [][]*x509.Certificate{{{Raw: []byte("foreign")}}}
			case "foreign-target":
				scope.TargetID = "foreign"
			case "foreign-tenant":
				scope.TenantID = "00000000-0000-0000-0000-000000000002"
			case "server-leaf":
				leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
			case "extra-uri":
				leaf.URIs = append(append([]*url.URL(nil), leaf.URIs...), leaf.URIs[0])
			case "expired":
				leaf.NotAfter = time.Now().Add(-time.Second)
			case "extra-dns":
				leaf.DNSNames = []string{"extra.example"}
			}
			before := registry.calls
			if err := AdmitTLSNode(context.Background(), candidate, scope, registry); !errors.Is(err, ErrDenied) || registry.calls != before {
				t.Fatal("TLS/scope drift reached registry admission", err)
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	before := registry.calls
	if !errors.Is(AdmitTLSNode(cancelled, state, testScope(), registry), ErrDenied) || registry.calls != before {
		t.Fatal("cancelled admission reached registry")
	}
}

func TestCertificateSerialNormalizationIsBounded(t *testing.T) {
	if serial, err := NormalizeCertificateSerial("00:AB:CD"); err != nil || serial != "abcd" {
		t.Fatal("issuer serial representation did not normalize", serial, err)
	}
	for _, serial := range []string{"", "00:00", "-1", "not-a-serial", "01 SECRET"} {
		if _, err := NormalizeCertificateSerial(serial); !errors.Is(err, ErrDenied) {
			t.Fatal("invalid serial admitted", serial)
		}
	}
}
