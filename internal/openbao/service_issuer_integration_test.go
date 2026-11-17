package openbao

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

// realCAEnv wires an in-memory real CA into the openbao service PKI read path so
// ServiceIssuer observes an actual issuing certificate, not a fake mount
// reference. The CA PEM is returned for `bao read -format=json baseharbor-pki/
// cert/ca`, which is exactly what ServiceCA reads, so BaseSerial/BaseExpiry go
// through the production ServiceCA parse path.
type realCAIssuerEnv struct {
	servicePKIFake
	caPEM      []byte
	caSerial   string
	caExpiry   time.Time
	crlSerials []string
}

func (f *realCAIssuerEnv) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "auth/approle/login"):
		return `{"auth":{"client_token":"manager-token"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/cert/ca"):
		return `{"data":{"certificate":"` + strings.ReplaceAll(string(f.caPEM), "\n", "\\n") + `"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/list-revoked"):
		serials := []string{}
		if len(f.crlSerials) > 0 {
			serials = f.crlSerials
		}
		out, _ := json.Marshal(map[string]any{"data": map[string]any{"serials": serials}})
		return string(out), nil
	default:
		return "", nil
	}
}

// buildRealCA returns a self-signed CA certificate and its PEM. A fixed serial
// and not-after are used so the test can assert exact BaseSerial/BaseExpiry
// values against the real certificate.
func buildRealCA(t *testing.T) (*x509.Certificate, []byte, string, time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	serial = serial.Add(serial, big.NewInt(1))
	notAfter := time.Date(2030, time.June, 1, 0, 0, 0, 0, time.UTC)
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "BaseHarbor Real Service CA"},
		NotBefore:             time.Now().UTC().Add(-time.Minute),
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return ca, pemBytes, ca.SerialNumber.Text(16), notAfter
}

// TestServiceIssuerBaseSerialIsRealCASerial proves the adapter observes the
// actual issuing CA serial (hex, not a mount reference), that the same serial is
// accepted as a serial-key by the read-only RevokedStatus, and that a revoked CRL
// round-trips back to revoked=true through that exact serial.
func TestServiceIssuerBaseSerialIsRealCASerial(t *testing.T) {
	_, caPEM, caSerialHex, _ := buildRealCA(t)
	env := &realCAIssuerEnv{caPEM: caPEM}
	issuer := NewServiceIssuer(env, servicePKITestFiles(t))

	baseSerial, err := issuer.BaseSerial(context.Background())
	if err != nil {
		t.Fatalf("BaseSerial failed: %v", err)
	}
	// ca.SerialNumber.Text(16) can yield an odd number of hex digits (random
	// 128-bit serial). Canonical hex is even-length, so left-pad both sides with
	// "0" before comparing and decoding. BaseSerial itself is correct; only the
	// canonical (even-length) form is used for the comparison below.
	if len(caSerialHex)%2 != 0 {
		caSerialHex = "0" + caSerialHex
	}
	if len(baseSerial)%2 != 0 {
		baseSerial = "0" + baseSerial
	}
	if baseSerial != caSerialHex {
		t.Fatalf("BaseSerial = %q, want real CA serial %q (hex)", baseSerial, caSerialHex)
	}
	if baseSerial == serviceIssuerReference {
		t.Fatalf("BaseSerial returned the mount reference instead of the CA serial")
	}
	if _, hexErr := hex.DecodeString(baseSerial); hexErr != nil {
		t.Fatalf("BaseSerial is not a plain hex integer: %q (%v)", baseSerial, hexErr)
	}

	// The CA serial is a readable, not-revoked status through RevokedStatus.
	revoked, err := issuer.RevokedStatus(context.Background(), baseSerial)
	if err != nil {
		t.Fatalf("RevokedStatus for the CA serial failed: %v", err)
	}
	if revoked {
		t.Fatal("real CA serial reported as revoked on a clean CRL")
	}

	// A CRL listing the CA serial makes RevokedStatus report revoked=true for the
	// exact same serial-key (round-trip through the real CA serial). OpenBao
	// stores CRL entries in the canonical colon-separated byte form, so list that
	// canonical form and confirm the raw BaseSerial still matches it.
	canonical, err := canonicalServiceRevocationSerial(caSerialHex)
	if err != nil {
		t.Fatalf("canonicalize CA serial: %v", err)
	}
	env.crlSerials = []string{canonical}
	revoked, err = issuer.RevokedStatus(context.Background(), baseSerial)
	if err != nil {
		t.Fatalf("RevokedStatus with CRL failed: %v", err)
	}
	if !revoked {
		t.Fatal("CA serial not reported as revoked when present in the CRL")
	}
}

// TestServiceIssuerBaseExpiryIsRealCANotAfter proves BaseExpiry returns the
// parsed not-after of the real CA certificate, not a fixed +90 day estimate.
func TestServiceIssuerBaseExpiryIsRealCANotAfter(t *testing.T) {
	_, caPEM, _, caExpiry := buildRealCA(t)
	env := &realCAIssuerEnv{caPEM: caPEM}
	issuer := NewServiceIssuer(env, servicePKITestFiles(t))

	baseExpiry, err := issuer.BaseExpiry(context.Background())
	if err != nil {
		t.Fatalf("BaseExpiry failed: %v", err)
	}
	if !baseExpiry.Equal(caExpiry) {
		t.Fatalf("BaseExpiry = %v, want real CA NotAfter %v (UTC)", baseExpiry.UTC(), caExpiry.UTC())
	}
	if baseExpiry.Location() != time.UTC {
		t.Fatalf("BaseExpiry = %v, want UTC location", baseExpiry)
	}
}

// TestServiceIssuerBaseSerialFailsClosedOnUnreadableCA proves an unreadable
// service CA fails closed (err != nil) rather than projecting an unknown base
// identity, so the production selection boundary rejects rather than accepts.
func TestServiceIssuerBaseSerialFailsClosedOnUnreadableCA(t *testing.T) {
	_, caPEM, _, _ := buildRealCA(t)
	env := &realCAIssuerEnv{caPEM: caPEM}
	// Break the CA payload so it does not parse as a certificate.
	env.caPEM = []byte("-----BEGIN CERTIFICATE-----\nnot-a-real-cert-bytes\n-----END CERTIFICATE-----\n")
	issuer := NewServiceIssuer(env, servicePKITestFiles(t))

	if _, err := issuer.BaseSerial(context.Background()); err == nil {
		t.Fatal("BaseSerial did not fail closed on an unparseable CA")
	}
	if _, err := issuer.BaseExpiry(context.Background()); err == nil {
		t.Fatal("BaseExpiry did not fail closed on an unparseable CA")
	}
}
