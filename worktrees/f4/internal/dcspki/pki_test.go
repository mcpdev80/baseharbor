package dcspki

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesMutualTLSPKI(t *testing.T) {
	dir := t.TempDir()
	f, err := Ensure(dir, []string{"etcd-1", "etcd-2", "etcd-3"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{f.CAKey, f.ServerKey, f.ClientKey} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", filepath.Base(path), st.Mode().Perm())
		}
	}
	data, err := os.ReadFile(f.ServerCert)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("server certificate PEM missing")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("etcd-2"); err != nil {
		t.Fatalf("server identity missing: %v", err)
	}
	if err := cert.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("loopback recovery identity missing: %v", err)
	}
	if len(cert.ExtKeyUsage) != 2 {
		t.Fatalf("expected server+client auth, got %v", cert.ExtKeyUsage)
	}
}

func TestEnsureRejectsPartialPKI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(dir, []string{"etcd-1"}); err == nil {
		t.Fatal("expected partial PKI rejection")
	}
}
