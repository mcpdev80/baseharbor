package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type coreSigningTestIssuer struct {
	serviceaccess.Issuer
	ca    *x509.Certificate
	caKey ed25519.PrivateKey
	bad   bool
}

func newCoreSigningTestIssuer(t *testing.T) *coreSigningTestIssuer {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(101),
		Subject:      pkix.Name{CommonName: "Core signing test CA"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(48 * time.Hour),
		IsCA:         true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &coreSigningTestIssuer{Issuer: serviceissuer.New(t), ca: parsed, caKey: key}
}

func (i *coreSigningTestIssuer) TrustBundle(context.Context) (serviceaccess.TrustBundle, error) {
	return serviceaccess.TrustBundle{
		PEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.ca.Raw}),
	}, nil
}

func (i *coreSigningTestIssuer) SignCoreCSR(_ context.Context, request serviceaccess.CSRSigningRequest) (serviceaccess.IssuedCertificate, error) {
	csr, err := request.ValidateCore()
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	now := time.Now()
	dns := csr.DNSNames
	usage := []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	if i.bad {
		usage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(202),
		Subject:      pkix.Name{CommonName: request.Identity},
		NotBefore:    now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: usage, DNSNames: dns, URIs: csr.URIs,
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, i.ca, csr.PublicKey, i.caKey)
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	return serviceaccess.IssuedCertificate{
		Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		IssuingCA:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.ca.Raw}),
		ExpiresAt:   certificate.NotAfter, Serial: certificate.SerialNumber.Text(16),
	}, nil
}

func TestManagedCoreConnectorTLSRequiresSPIFFEAndServerAuthOnly(t *testing.T) {
	root := t.TempDir()
	issuer := newCoreSigningTestIssuer(t)
	installationID := "6a185ba5-003b-4708-8c91-aafa7d369e53"
	result, err := issueManagedCoreConnectorTLS(t.Context(), issuer, root, installationID, "node.docker-kilo.mcp-dev.de")
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.LoadX509KeyPair(result.Certificate, result.Key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	identity := "spiffe://baseharbor/platform/core/" + installationID
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != identity ||
		len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth ||
		leaf.VerifyHostname("node.docker-kilo.mcp-dev.de") != nil {
		t.Fatalf("Core Connector identity violates serving contract: %+v", leaf)
	}
	for _, file := range []string{result.Certificate, result.Key, result.CA} {
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("Core Connector serving file not private: %s: %v", file, err)
		}
		if filepath.Dir(file) != filepath.Join(root, "managed-serving", "core-connector") {
			t.Fatal("Core Connector serving material escaped owned state")
		}
	}
	issuer.bad = true
	if _, err := issueManagedCoreConnectorTLS(t.Context(), issuer, root, installationID, "node.docker-kilo.mcp-dev.de"); err == nil {
		t.Fatal("dual server/client EKU was accepted")
	}
	if _, err := issueManagedCoreConnectorTLS(t.Context(), issuer, root, installationID, "192.168.6.178"); err == nil {
		t.Fatal("unauthorized IP SAN was accepted by DNS-only Core CSR")
	}
	if _, err := issueManagedCoreConnectorTLS(t.Context(), issuer, root, "../foreign", "node.docker-kilo.mcp-dev.de"); err == nil {
		t.Fatal("foreign Core SPIFFE identity was admitted")
	}
}

var _ serviceaccess.CoreCSRIssuer = (*coreSigningTestIssuer)(nil)
