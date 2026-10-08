package serviceaccess

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/url"
	"testing"
	"time"
)

func TestCSRScopeSignatureAndSingleObjectValidation(t *testing.T) {
	identity := "spiffe://baseharbor/platform/connectors/lab/node-a"
	uri, _ := url.Parse(identity)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	create := func(extraDNS bool) []byte {
		template := &x509.CertificateRequest{Subject: pkix.Name{CommonName: identity}, URIs: []*url.URL{uri}}
		if extraDNS {
			template.DNSNames = []string{"foreign.example"}
		}
		der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	}
	valid := CSRSigningRequest{CSRPEM: create(false), Identity: identity, TTL: time.Hour}
	if _, err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []CSRSigningRequest{
		{CSRPEM: append(create(false), create(false)...), Identity: identity, TTL: time.Hour},
		{CSRPEM: create(true), Identity: identity, TTL: time.Hour},
		{CSRPEM: create(false), Identity: identity + "foreign", TTL: time.Hour},
		{CSRPEM: create(false), Identity: identity, TTL: 25 * time.Hour},
		{CSRPEM: []byte("private-key-must-not-be-echoed"), Identity: identity, TTL: time.Hour},
	} {
		if _, err := bad.Validate(); err == nil {
			t.Fatal("unsafe CSR admitted")
		}
	}
}

func TestCoreCSRServerNamesRequireExplicitBoundedAuthority(t *testing.T) {
	identity := "spiffe://baseharbor/platform/core/primary"
	uri, _ := url.Parse(identity)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	create := func(names []string) CSRSigningRequest {
		t.Helper()
		der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: identity}, URIs: []*url.URL{uri}, DNSNames: names}, key)
		if err != nil {
			t.Fatal(err)
		}
		return CSRSigningRequest{Identity: identity, CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), TTL: time.Hour, DNSNames: names}
	}
	valid := create([]string{"core.example", "localhost"})
	if _, err := valid.ValidateCore(); err != nil {
		t.Fatal(err)
	}
	if _, err := valid.Validate(); err == nil {
		t.Fatal("node validation granted server-name authority")
	}
	unbound := valid
	unbound.DNSNames = nil
	if _, err := unbound.ValidateCore(); err == nil {
		t.Fatal("CSR self-authorized its server names")
	}
	for _, names := range [][]string{{"*"}, {"*.example"}, {"Core.example"}, {"core.example."}, {"-core.example"}, {"core..example"}, {"127.0.0.1"}, {"core.example", "core.example"}, {""}, {"a", "b", "c", "d", "e", "f", "g", "h", "i"}} {
		if _, err := create(names).ValidateCore(); err == nil {
			t.Fatalf("unapproved Core DNS authorization admitted: %q", names)
		}
	}
}
