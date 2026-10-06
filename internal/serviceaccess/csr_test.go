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
