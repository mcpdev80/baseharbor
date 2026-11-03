package managementaccess_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// readFixtureFile reads a testdata file, falling back to the canonical path when
// the test runs from the package parent directory.
func readFixtureFile(t *testing.T, name string) ([]byte, error) {
	if data, err := os.ReadFile(name); err == nil {
		return data, nil
	}
	return os.ReadFile(filepath.Join("..", "..", "managementaccess", "v1", name))
}

// generateTestCA creates a non-secret test CA certificate and returns its PEM
// and authoritative SHA-256 fingerprint (SHA-256 of the DER, matching
// hosttrust.ParseCA). No real credential is produced; the key never leaves this
// process and is not exported.
func generateTestCA(t *testing.T, name string) (pemPEM string, sha256sum string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
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
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}
	pemPEM = string(pem.EncodeToMemory(block))
	sum := sha256.Sum256(der)
	return pemPEM, hex.EncodeToString(sum[:])
}

// authorizedScope returns a valid enrollment scope bound to a recipient that a
// management record must carry to be considered non-foreign.
func authorizedScope() targetenrollment.Scope {
	return targetenrollment.Scope{
		TenantID: "00000000-0000-0000-0000-000000000001",
		TargetID: "lab",
		NodeID:   "node-a",
		Runtime:  "docker",
	}
}

// managementFixturesCA returns the non-secret test CA PEM shared by the golden
// fixtures. The private key never leaves this process; only the public
// certificate is exported, so no real credential is produced.
func managementFixturesCA(t *testing.T) string {
	t.Helper()
	ca, _ := generateTestCA(t, "BaseHarbor Management r27 Test CA")
	return ca
}

// managementFixturesCAFA returns the authoritative SHA-256 fingerprint (SHA-256
// of the DER, matching hosttrust.ParseCA) of the shared fixture CA.
func managementFixturesCAFA(t *testing.T) string {
	t.Helper()
	_, sha := generateTestCA(t, "BaseHarbor Management r27 Test CA")
	return sha
}

// readGoldenFixtures loads the schema golden fixtures for the complementary
// schema-validation test. It does not assert behavioral enforcement; the
// behavioral negatives live in managementaccess_test.go.
func readGoldenFixtures(t *testing.T) []byte {
	t.Helper()
	data, err := readFixtureFile(t, "profile.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// profileFor returns a deep clone of the base profile with the given overrides.
func profileFor(t *testing.T) managementaccess.ManagementAccessProfile {
	t.Helper()
	caPEM, caSHA := generateTestCA(t, "BaseHarbor Management Test CA")
	now := time.Now().UTC()
	return managementaccess.ManagementAccessProfile{
		Kind:           managementaccess.ManagementProfile,
		Version:        "baseharbor.management-access/v1",
		InstallationID: "11111111-1111-4111-8111-111111111111",
		Trust: managementaccess.Trust{
			CAPEM:  caPEM,
			SHA256: caSHA,
		},
		Recipient: "spiffe://baseharbor/platform/core/00000000-0000-4000-8000-000000000001/homelab",
		Scope:     "control-plane",
		NotBefore: now.Add(-time.Minute),
		NotAfter:  now.Add(time.Hour),
		Import:    managementaccess.ImportCapability{RightsGranting: false, PluginExecution: false, CredentialRedirect: false},
		Storage:   managementaccess.Storage{Class: managementaccess.ProtectedDurable, Durable: true, Encrypted: true, DisasterRecovery: false},
	}
}
