package serviceaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalIssuerStableAuthorityAndActualTLS(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pki")
	i, err := EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := i.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reopened.TrustBundle(context.Background())
	if err != nil || string(second.PEM) != string(trust.PEM) || second.IssuerReference != trust.IssuerReference {
		t.Fatal("issuer authority changed on repeat apply", err)
	}
	leaf, err := i.Issue(context.Background(), CertificateRequest{CommonName: "database", DNSNames: []string{"database.localhost"}, TTL: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(leaf.Certificate, leaf.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(trust.PEM)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "database.localhost"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "foreign.localhost"}); err == nil {
		t.Fatal("foreign name accepted")
	}
	if _, err := i.Renew(context.Background(), leaf, CertificateRequest{CommonName: "database", DNSNames: []string{"database.localhost"}, TTL: time.Hour}); err != nil {
		t.Fatal(err)
	}
	leaf.IssuerReference = "foreign"
	if _, err := i.Renew(context.Background(), leaf, CertificateRequest{CommonName: "database", TTL: time.Hour}); err == nil {
		t.Fatal("foreign issuer renewed")
	}
}

func TestLocalIssuerReadNeverInitializesAndCorruptionNeverRegenerates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pki")
	i := &LocalIssuer{Dir: dir}
	if _, err := i.Status(context.Background()); err == nil {
		t.Fatal("missing authority reported ready")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read created issuer state")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "issuer.pem")
	if err := os.WriteFile(path, []byte("damaged authority"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLocalIssuer(dir); err == nil {
		t.Fatal("damaged authority replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "damaged authority" {
		t.Fatal("damaged authority changed")
	}
}

func TestLocalIssuerRejectsSymlinkAndPublicAuthority(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pki")
	i, err := EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "issuer.pem")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Status(context.Background()); err == nil {
		t.Fatal("public private key accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLocalIssuer(link); err == nil {
		t.Fatal("symlink authority accepted")
	}
}

func TestLocalIssuerSignedPersistentIdempotentRevocation(t *testing.T) {
	i, err := EnsureLocalIssuer(filepath.Join(t.TempDir(), "pki"))
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := i.Issue(context.Background(), CertificateRequest{CommonName: "client", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Revoke(context.Background(), leaf.Serial); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(i.Dir, "revocations.pem")
	before, _ := os.ReadFile(path)
	if err := i.Revoke(context.Background(), leaf.Serial); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("duplicate revocation changed CRL")
	}
	block, _ := pem.Decode(after)
	list, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ca, _, _, err := i.load()
	if err != nil || list.CheckSignatureFrom(ca) != nil || len(list.RevokedCertificateEntries) != 1 {
		t.Fatal("unverifiable revocation", err)
	}
	if _, err := i.Renew(context.Background(), leaf, CertificateRequest{CommonName: "client", TTL: time.Hour}); err == nil {
		t.Fatal("revoked certificate renewed")
	}
	if err := i.Revoke(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid serial accepted")
	}
}
