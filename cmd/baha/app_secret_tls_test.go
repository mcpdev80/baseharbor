package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestParseTLSSetArgs(t *testing.T) {
	name, cert, key, chain, err := parseTLSSetArgs([]string{"demo", "--cert-file", "cert.pem", "--key-file=key.pem", "--chain-file", "chain.pem"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || cert != "cert.pem" || key != "key.pem" || chain != "chain.pem" {
		t.Fatalf("unexpected parse result: %q %q %q %q", name, cert, key, chain)
	}
	if _, _, _, _, err := parseTLSSetArgs([]string{"--cert-file", "cert.pem"}); err == nil {
		t.Fatal("expected missing key file to fail")
	}
}

func TestNormalizeCertificateInputSupportsPEMAndDER(t *testing.T) {
	certPEM, _, certDER := testTLSMaterial(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	for _, input := range [][]byte{certPEM, certDER} {
		normalized, leaf, err := normalizeCertificateInput(input)
		if err != nil {
			t.Fatal(err)
		}
		if leaf == nil || len(normalized) == 0 {
			t.Fatal("certificate normalization returned empty material")
		}
		block, _ := pem.Decode(normalized)
		if block == nil || block.Type != "CERTIFICATE" {
			t.Fatal("normalized certificate is not PEM")
		}
	}
}

func TestTLSMaterialPairMatchesAndValidityIsInspectable(t *testing.T) {
	certPEM, keyPEM, _ := testTLSMaterial(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	_, leaf, err := normalizeCertificateInput(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		t.Fatal("generated test certificate is unexpectedly outside its validity window")
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("generated certificate/key pair does not match: %v", err)
	}

	_, wrongKey, _ := testTLSMaterial(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if _, err := tls.X509KeyPair(certPEM, wrongKey); err == nil {
		t.Fatal("expected mismatched private key to fail")
	}
}

func testTLSMaterial(t *testing.T, notBefore, notAfter time.Time) (certPEM, keyPEM, certDER []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err = x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, certDER
}
