package openbao

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestManagedCoreCSRPreservesExplicitAuthorizedServerName(t *testing.T) {
	identity := "spiffe://baseharbor/platform/core/primary"
	uri, _ := url.Parse(identity)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: identity}, URIs: []*url.URL{uri}, DNSNames: []string{"core.example"},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	request := serviceaccess.CSRSigningRequest{Identity: identity, CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), TTL: time.Hour, DNSNames: []string{"core.example"}}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: identity}, URIs: []*url.URL{uri}, DNSNames: []string{"core.example"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	reply := func() string {
		t.Helper()
		der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, pub, key)
		if err != nil {
			t.Fatal(err)
		}
		certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		data, err := json.Marshal(map[string]any{"data": map[string]any{"certificate": string(certificate), "issuing_ca": string(certificate), "serial_number": "01"}})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	fake := &csrPKIFake{reply: reply()}
	issuer := NewServiceIssuer(fake, servicePKITestFiles(t))
	issued, err := issuer.SignCoreCSR(context.Background(), request)
	if err != nil {
		t.Fatalf("managed Core certificate cannot satisfy authenticated TLS server-name verification: %v", err)
	}
	block, _ := pem.Decode(issued.Certificate)
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil || certificate.VerifyHostname("core.example") != nil || len(issued.PrivateKey) != 0 {
		t.Fatal("server identity or key locality differs", err)
	}
	if _, err := issuer.SignCSR(context.Background(), request); err == nil {
		t.Fatal("node signer obtained server-name authority")
	}
	for _, names := range [][]string{{"foreign.example"}, {"core.example", "foreign.example"}, {}} {
		leaf.DNSNames = names
		fake.reply = reply()
		if _, err := issuer.SignCoreCSR(context.Background(), request); err == nil {
			t.Fatal("signed names escaped explicit Core authorization")
		}
	}
	unbound := request
	unbound.DNSNames = nil
	if _, err := issuer.SignCoreCSR(context.Background(), unbound); err == nil {
		t.Fatal("CSR granted unapproved server-name authority")
	}
}
