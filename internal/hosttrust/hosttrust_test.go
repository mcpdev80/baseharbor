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

func TestRemoveOwnedDetailedMultipleAnchorsPreservesForeignReplacement(t *testing.T) {
	stateDir := t.TempDir()
	root := t.TempDir()
	backend := &fakeBackend{root: root, name: "fake"}
	original := resolveBackend
	resolveBackend = func(name string) (Backend, error) {
		if name == "fake" {
			return backend, nil
		}
		return original(name)
	}
	defer func() { resolveBackend = original }()
	a, err := Install(context.Background(), stateDir, testCA(t, "owned A"), "local", backend)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Install(context.Background(), stateDir, testCA(t, "owned B"), "local", backend)
	if err != nil {
		t.Fatal(err)
	}
	foreign := testCA(t, "operator replacement")
	if err := os.WriteFile(b.Path, foreign, 0644); err != nil {
		t.Fatal(err)
	}
	report, err := RemoveOwnedDetailed(context.Background(), stateDir)
	if err == nil {
		t.Fatal("fingerprint replacement not refused")
	}
	if len(report.Removed) != 1 || len(report.Preserved) != 1 || report.Preserved[0].Fingerprint != b.Fingerprint {
		t.Fatalf("unexpected results %+v", report)
	}
	if _, err := os.Stat(a.Path); !os.IsNotExist(err) {
		t.Fatalf("owned CA not removed: %v", err)
	}
	if _, err := os.Stat(b.Path); err != nil {
		t.Fatalf("foreign CA deleted: %v", err)
	}
	records, err := StateRecords(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Fingerprint != b.Fingerprint {
		t.Fatalf("PRESERVED ownership lost: %+v", records)
	}
	if _, err := RemoveOwnedDetailed(context.Background(), stateDir); err == nil {
		t.Fatal("repeat must still preserve foreign CA")
	}
}

func TestRemoveOwnedDetailedIdempotentAndSymlinkSafe(t *testing.T) {
	stateDir := t.TempDir()
	root := t.TempDir()
	backend := &fakeBackend{root: root, name: "fake"}
	old := resolveBackend
	resolveBackend = func(n string) (Backend, error) {
		if n == "fake" {
			return backend, nil
		}
		return old(n)
	}
	defer func() { resolveBackend = old }()
	st, err := Install(context.Background(), stateDir, testCA(t, "owned link"), "local", backend)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "foreign.crt")
	if err := os.WriteFile(source, testCA(t, "foreign"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(st.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, st.Path); err != nil {
		t.Fatal(err)
	}
	report, err := RemoveOwnedDetailed(context.Background(), stateDir)
	if err == nil || len(report.Preserved) != 1 {
		t.Fatalf("symlink not preserved: %+v %v", report, err)
	}
	if _, err := os.Lstat(st.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(st.Path); err != nil {
		t.Fatal(err)
	}
	report, err = RemoveOwnedDetailed(context.Background(), stateDir)
	if err != nil || len(report.Removed) != 1 {
		t.Fatalf("idempotent removal failed: %+v %v", report, err)
	}
	report, err = RemoveOwnedDetailed(context.Background(), stateDir)
	if err != nil || len(report.Removed) != 0 {
		t.Fatalf("second removal not idempotent: %+v %v", report, err)
	}
}

func TestUntrackedCandidatesArePreservedNotClaimed(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	file := filepath.Join(dir, "baseharbor-0123456789abcdef.crt")
	if err := os.WriteFile(file, testCA(t, "orphan"), 0644); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "operator.crt")
	if err := os.WriteFile(foreign, testCA(t, "operator"), 0644); err != nil {
		t.Fatal(err)
	}
	candidates, err := findUntrackedCandidates(stateDir, []struct{ path, extension string }{{dir, ".crt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0] != file {
		t.Fatalf("unexpected untracked list: %v", candidates)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("orphan removed: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign anchor removed: %v", err)
	}
}

func TestSystemHostTrustBackendsRefreshCorrectStore(t *testing.T){
 cases:=[]struct{name,expected string}{
  {"linux-update-ca-certificates","update-ca-certificates"},
  {"linux-update-ca-trust","update-ca-trust"},
 }
 for _,tc:=range cases{
  backend,err:=backendByName(tc.name)
  if err!=nil{t.Fatal(err)}
  actual:=backend.(*systemBackend)
  if len(actual.refreshCmd)==0||actual.refreshCmd[0]!=tc.expected{t.Fatalf("backend %s does not refresh system trust: %v",tc.name,actual.refreshCmd)}
 }
}
