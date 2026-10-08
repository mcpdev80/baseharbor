package serviceaccess_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestSignedCSRRejectsForeignAuthorityKeyAndUntrustedMetadata(t *testing.T) {
	identity := "spiffe://baseharbor/platform/connectors/tenant/target/node"
	uri, _ := url.Parse(identity)
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	raw, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uri}}, key)
	if err != nil {
		t.Fatal(err)
	}
	request := serviceaccess.CSRSigningRequest{Identity: identity, CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: raw}), TTL: time.Hour}
	authority := serviceissuer.New(t)
	issued, err := authority.SignCSR(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	trust, _ := authority.TrustBundle(context.Background())
	if err := serviceaccess.ValidateSignedCSR(request, issued, trust, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"foreign-authority", "private-key", "serial", "expiry", "leading-pem-garbage", "trailing-pem", "trust-garbage", "foreign-key"} {
		t.Run(change, func(t *testing.T) {
			material, roots, signing := issued, trust, request
			switch change {
			case "foreign-authority":
				roots, _ = serviceissuer.New(t).TrustBundle(context.Background())
			case "private-key":
				material.PrivateKey = []byte("PRIVATE")
			case "serial":
				material.Serial = "ffff"
			case "expiry":
				material.ExpiresAt = material.ExpiresAt.Add(time.Second)
			case "leading-pem-garbage":
				material.Certificate = append([]byte("SECRET\n"), material.Certificate...)
			case "trailing-pem":
				material.Certificate = append(append([]byte(nil), material.Certificate...), material.Certificate...)
			case "trust-garbage":
				roots.PEM = append([]byte("SECRET\n"), roots.PEM...)
			case "foreign-key":
				_, foreign, _ := ed25519.GenerateKey(rand.Reader)
				other, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uri}}, foreign)
				signing.CSRPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: other})
			}
			if err := serviceaccess.ValidateSignedCSR(signing, material, roots, time.Now()); err == nil {
				t.Fatal("invalid authority material accepted")
			}
		})
	}
}
