package serviceissuer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// TestBaseSerialIsRealCAHexAndBaseExpiryIsRealCertNotAfter proves the
// testsupport/OpenBao issuer reports the genuine CA serial (hex, no colon) — the
// format RevokedStatus compares the CRL against — and the genuine CA NotAfter, so
// a leaf serial is comparable to the CRL at the serial level and the expiry bound
// is the real certificate lifetime, not an invented window.
func TestBaseSerialIsRealCAHexAndBaseExpiryIsRealCertNotAfter(t *testing.T) {
	iss := New(t)
	baseSerial, err := iss.BaseSerial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	baseExpiry, err := iss.BaseExpiry(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// The reported serial must be the CA's real serial number in hex (lowercase,
	// no colons) — not the openbao:// URI.
	if baseSerial == "" {
		t.Fatal("BaseSerial returned empty")
	}
	if strings.Contains(baseSerial, ":") {
		t.Fatalf("BaseSerial not a plain hex serial: %q", baseSerial)
	}
	if got := iss.ca.SerialNumber.Text(16); got != baseSerial {
		t.Fatalf("BaseSerial %q != real CA serial %q", baseSerial, got)
	}
	// The reported expiry must equal the real CA NotAfter, not an invented window.
	if !baseExpiry.Equal(iss.ca.NotAfter) {
		t.Fatalf("BaseExpiry %v != real CA NotAfter %v", baseExpiry, iss.ca.NotAfter)
	}

	// The issued leaf must carry the real CA serial as its Reference, so a
	// RevokedStatus comparison is a serial-level CRL match.
	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "client", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Reference != iss.ca.SerialNumber.Text(16) {
		t.Fatalf("issued leaf Reference %q != real CA serial %q", leaf.Reference, iss.ca.SerialNumber.Text(16))
	}
	if !leaf.IssuerNotAfter.Equal(iss.ca.NotAfter) {
		t.Fatalf("issued leaf IssuerNotAfter %v != real CA NotAfter %v", leaf.IssuerNotAfter, iss.ca.NotAfter)
	}
}
