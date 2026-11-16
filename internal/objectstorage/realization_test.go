package objectstorage

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadSeaweedFSTrustBundlePositive(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
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
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	want := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readSeaweedFSTrustBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("trust bundle changed during read")
	}
}

func TestReadSeaweedFSTrustBundleRejectsPathTextNegative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("/tmp/provider/ca.pem\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSeaweedFSTrustBundle(path); err == nil {
		t.Fatal("expected invalid PEM trust bundle to fail")
	}
}

func TestReadSeaweedFSTrustBundleRejectsMissingFileNegative(t *testing.T) {
	if _, err := readSeaweedFSTrustBundle(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Fatal("expected missing trust bundle to fail")
	}
}
