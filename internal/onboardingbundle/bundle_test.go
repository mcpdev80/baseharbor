package onboardingbundle

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validBundle(t *testing.T) Bundle {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test core ca"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}, &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test core ca"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return Bundle{
		ContractVersion:    Contract,
		CoreURL:            "https://192.0.2.8:18443",
		CAPEM:              string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		InstallationID:     "11111111-1111-4111-8111-111111111111",
		CoreInstallationID: "33333333-3333-4333-8333-333333333333",
		RuntimeProvider:    "docker",
		Station:            "workstation-a",
		Token:              strings.Repeat("T", 48),
		Nonce:              strings.Repeat("N", 48),
		ExpiresAt:          now.Add(5 * time.Minute),
	}
}

func TestBundleValidationFailsClosed(t *testing.T) {
	now := time.Now()
	b := validBundle(t)
	if err := b.Validate(now); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	cases := map[string]func(*Bundle){
		"expired":            func(b *Bundle) { b.ExpiresAt = now.Add(-time.Second) },
		"excessive lifetime": func(b *Bundle) { b.ExpiresAt = now.Add(30 * time.Minute) },
		"http":               func(b *Bundle) { b.CoreURL = "http://192.0.2.8:18443" },
		"userinfo":           func(b *Bundle) { b.CoreURL = "https://user:password@192.0.2.8:18443" },
		"fragment":           func(b *Bundle) { b.CoreURL += "#secret" },
		"foreign tenant":     func(b *Bundle) { b.InstallationID = "tenant-other" },
		"invalid station":    func(b *Bundle) { b.Station = "../foreign" },
		"missing CA":         func(b *Bundle) { b.CAPEM = "" },
		"private key":        func(b *Bundle) { b.CAPEM += "\nPRIVATE KEY" },
		"missing grant":      func(b *Bundle) { b.Token = "" },
		"wrong version":      func(b *Bundle) { b.ContractVersion = "unknown" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := b
			mutate(&candidate)
			if !errors.Is(candidate.Validate(now), ErrInvalid) {
				t.Fatal("unsafe bundle accepted")
			}
		})
	}
}

func TestReadRequiresPrivateRegularSingleJSON(t *testing.T) {
	now := time.Now()
	b := validBundle(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "invite.json")
	payload, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, now); err != nil {
		t.Fatalf("valid invite: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("world-readable invite accepted: %v", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(payload, []byte(" {}")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second JSON accepted: %v", err)
	}
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(filepath.Join(dir, "link.json"), now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("symlink accepted: %v", err)
	}
}

func TestWriteSecureInvitationRefusesOverwrite(t *testing.T) {
	now := time.Now()
	b := validBundle(t)
	privateDir := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(privateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateDir, "onboard.json")
	if err := Write(path, b, now); err != nil {
		t.Fatalf("write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %v", fi.Mode())
	}
	if got, err := Read(path, now); err != nil || got.CoreURL != b.CoreURL {
		t.Fatalf("roundtrip failed: %+v %v", got, err)
	}
	if err := Write(path, b, now); err == nil {
		t.Fatal("overwrote existing bearer grant")
	}
	publicDir := filepath.Join(t.TempDir(), "public")
	if err := os.Mkdir(publicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(publicDir, "onboard.json"), b, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe output directory: %v", err)
	}
}
