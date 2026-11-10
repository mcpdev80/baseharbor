package openbao

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestSignedCRLFallbackVerifiesFreshnessIssuerAndRevocations(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{CommonName: "Test OpenBao CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	create := func(template *x509.RevocationList) string {
		t.Helper()
		der, err := x509.CreateRevocationList(rand.Reader, template, ca, key)
		if err != nil {
		t.Fatal(err)
	}
		encoded, err := json.Marshal(map[string]any{"data": map[string]string{
			"certificate": string(pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})),
		}})
		if err != nil {
		t.Fatal(err)
	}
		return string(encoded)
	}
	base := &x509.RevocationList{
		Number:     big.NewInt(1),
		ThisUpdate: now.Add(-time.Minute),
		NextUpdate: now.Add(time.Hour),
	}
	response := create(base)
	revoked, err := revokedSerialInSignedCRL(response, "01:02", ca, now)
	if err != nil || revoked {
		t.Fatalf("trusted fresh empty CRL rejected: revoked=%v err=%v", revoked, err)
	}
	base.RevokedCertificateEntries = []x509.RevocationListEntry{{
		SerialNumber:   big.NewInt(0x102),
		RevocationTime: now.Add(-time.Minute),
	}}
	response = create(base)
	revoked, err = revokedSerialInSignedCRL(response, "01:02", ca, now)
	if err != nil || !revoked {
		t.Fatalf("signed revocation not enforced: revoked=%v err=%v", revoked, err)
	}
	if _, err := revokedSerialInSignedCRL(response, "01:02", ca, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired CRL accepted")
	}
	if _, err := revokedSerialInSignedCRL(`{"data":{"certificate":"garbage"}}`, "01:02", ca, now); err == nil {
		t.Fatal("unsigned CRL accepted")
	}
	if _, err := revokedSerialInSignedCRL(`{"data":{}}`, "01:02", ca, now); err == nil {
		t.Fatal("missing CRL accepted")
	}
	secondKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secondDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &secondKey.PublicKey, secondKey)
	if err != nil {
		t.Fatal(err)
	}
	secondCA, err := x509.ParseCertificate(secondDER)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revokedSerialInSignedCRL(response, "01:02", secondCA, now); err == nil {
		t.Fatal("CRL signed by foreign CA accepted")
	}
}
