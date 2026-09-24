package hosttrust

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
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

type fakeBackend struct {
	root string
	name string
}

func (b *fakeBackend) Name() string { return b.name }

func (b *fakeBackend) AnchorPath(fingerprint string) (string, error) {
	return filepath.Join(b.root, "baseharbor-"+fingerprint[:16]+".crt"), nil
}

func (b *fakeBackend) Install(_ context.Context, source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func (b *fakeBackend) Remove(_ context.Context, target string) error {
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func TestExportWritesOnlyPublicCA(t *testing.T) {
	ca := testCA(t, "BaseHarbor test CA")
	path := filepath.Join(t.TempDir(), "baseharbor-ca.pem")
	if err := Export(path, ca); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 {
		t.Fatalf("export must contain exactly one public certificate block")
	}
}

func TestExportRefusesOverwrite(t *testing.T) {
	ca := testCA(t, "BaseHarbor export CA")
	path := filepath.Join(t.TempDir(), "baseharbor-ca.pem")
	if err := os.WriteFile(path, []byte("operator-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Export(path, ca); err == nil {
		t.Fatal("expected existing export destination to fail closed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "operator-owned" {
		t.Fatal("existing export destination was modified")
	}
}

func TestInstallAndRemoveOwnedTrust(t *testing.T) {
	stateDir := t.TempDir()
	anchors := t.TempDir()
	backend := &fakeBackend{root: anchors, name: "fake"}
	ca := testCA(t, "BaseHarbor owned CA")

	status, err := Install(context.Background(), stateDir, ca, "test://issuer", backend)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Owned || status.Path == "" {
		t.Fatalf("expected BaseHarbor-owned trust status: %+v", status)
	}
	if _, err := os.Stat(status.Path); err != nil {
		t.Fatalf("installed anchor missing: %v", err)
	}
	records, err := StateRecords(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Fingerprint != status.Fingerprint {
		t.Fatalf("unexpected ownership state: %+v", records)
	}

	oldResolver := resolveBackend
	resolveBackend = func(name string) (Backend, error) {
		if name == "fake" {
			return backend, nil
		}
		return oldResolver(name)
	}
	defer func() { resolveBackend = oldResolver }()

	removed, err := RemoveOwned(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d want 1", removed)
	}
	if _, err := os.Stat(status.Path); !os.IsNotExist(err) {
		t.Fatalf("owned anchor still exists: %v", err)
	}
}

func TestRemoveOwnedRefusesChangedAnchor(t *testing.T) {
	stateDir := t.TempDir()
	anchors := t.TempDir()
	backend := &fakeBackend{root: anchors, name: "fake"}
	ca := testCA(t, "BaseHarbor owned CA")
	status, err := Install(context.Background(), stateDir, ca, "test://issuer", backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(status.Path, testCA(t, "replacement CA"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldResolver := resolveBackend
	resolveBackend = func(name string) (Backend, error) {
		if name == "fake" {
			return backend, nil
		}
		return oldResolver(name)
	}
	defer func() { resolveBackend = oldResolver }()

	if _, err := RemoveOwned(context.Background(), stateDir); err == nil {
		t.Fatal("expected fingerprint mismatch to fail closed")
	}
	if _, err := os.Stat(status.Path); err != nil {
		t.Fatalf("changed anchor was removed: %v", err)
	}
}

func testCA(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
