package externalprovider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverCertificateDirectoryMatchesWildcardAndKey(t *testing.T) {
	dir := t.TempDir()
	ca, caKey := writeTestCA(t, dir)
	writeTestLeaf(t, dir, "server", ca, caKey, []string{"*.example.com"}, x509.ExtKeyUsageServerAuth)

	got, err := DiscoverCertificateDirectory(dir, "app.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CAReferences) != 1 {
		t.Fatalf("CAReferences=%#v", got.CAReferences)
	}
	if len(got.HostnameMatches) != 1 {
		t.Fatalf("HostnameMatches=%#v", got.HostnameMatches)
	}
	if got.HostnameMatches[0].Certificate == "" || got.HostnameMatches[0].PrivateKey == "" {
		t.Fatalf("pair=%#v", got.HostnameMatches[0])
	}
}

func TestDiscoverCertificateDirectoryFailsClosedOnAmbiguousHostname(t *testing.T) {
	dir := t.TempDir()
	ca, caKey := writeTestCA(t, dir)
	writeTestLeaf(t, dir, "server-a", ca, caKey, []string{"db.example.com"}, x509.ExtKeyUsageServerAuth)
	writeTestLeaf(t, dir, "server-b", ca, caKey, []string{"db.example.com"}, x509.ExtKeyUsageServerAuth)

	if _, err := DiscoverCertificateDirectory(dir, "db.example.com"); err == nil {
		t.Fatal("expected ambiguous matching certificates to fail")
	}
}

func writeTestCA(t *testing.T, dir string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	writePEMFile(t, filepath.Join(dir, "ca.pem"), "CERTIFICATE", der, 0o644)
	return cert, key
}

func writeTestLeaf(t *testing.T, dir, name string, ca *x509.Certificate, caKey *rsa.PrivateKey, dns []string, usage x509.ExtKeyUsage) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: dns[0]},
		DNSNames:     dns,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	writePEMFile(t, filepath.Join(dir, name+".pem"), "CERTIFICATE", der, 0o644)
	writePEMFile(t, filepath.Join(dir, name+".key"), "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600)
}

func writePEMFile(t *testing.T, path, typ string, der []byte, mode os.FileMode) {
	t.Helper()
	data := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
