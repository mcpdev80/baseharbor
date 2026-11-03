package serviceaccess

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
)

// fakeIssuer implements the production serviceaccess.Issuer for adapter tests.
type fakeIssuer struct {
	ready  bool
	revErr error
}

func (f *fakeIssuer) TrustBundle(ctx context.Context) (TrustBundle, error)     { return TrustBundle{}, nil }
func (f *fakeIssuer) Issue(ctx context.Context, r CertificateRequest) (IssuedCertificate, error) {
	return IssuedCertificate{}, nil
}
func (f *fakeIssuer) Renew(ctx context.Context, c IssuedCertificate, r CertificateRequest) (IssuedCertificate, error) {
	return IssuedCertificate{}, nil
}
func (f *fakeIssuer) Revoke(ctx context.Context, serial string) error { return f.revErr }
func (f *fakeIssuer) Status(ctx context.Context) (IssuerStatus, error) {
	return IssuerStatus{Ready: f.ready}, nil
}

func TestIssuerAdapterProjectsStatus(t *testing.T) {
	a := NewIssuerAdapter(&fakeIssuer{ready: true})
	status, err := a.Status(context.Background())
	if err != nil {
		t.Fatalf("adapter status error: %v", err)
	}
	if !status.Ready {
		t.Fatalf("adapter did not project readiness")
	}
	if status.IssuerReference != "" {
		t.Fatalf("unexpected issuer reference: %q", status.IssuerReference)
	}
}

func TestIssuerAdapterProjectsRevoke(t *testing.T) {
	want := context.Canceled
	a := NewIssuerAdapter(&fakeIssuer{revErr: want})
	if err := a.Revoke(context.Background(), "ABC"); err != want {
		t.Fatalf("adapter Revoke did not project underlying error")
	}
}

func TestIssuerAdapterSatisfiesLeafInterface(t *testing.T) {
	var _ installauthority.Issuer = NewIssuerAdapter(&fakeIssuer{ready: true})
}

func TestNewIssuerAdapterNilSafeStatus(t *testing.T) {
	// A nil production issuer must surface a boundary error, not panic.
	a := NewIssuerAdapter(nil)
	if _, err := a.Status(context.Background()); err == nil {
		t.Fatalf("nil issuer Status should error")
	}
}

func TestLeafSentinelIdentity(t *testing.T) {
	// serviceaccess sentinels must be the same values as the installauthority
	// leaf sentinels (re-exported), so callers on either side see one error.
	if ErrMissingAuthority != installauthority.ErrMissingAuthority {
		t.Fatalf("ErrMissingAuthority mismatch")
	}
	if ErrForeignAuthority != installauthority.ErrForeignAuthority {
		t.Fatalf("ErrForeignAuthority mismatch")
	}
	if ErrNotReady != installauthority.ErrNotReady {
		t.Fatalf("ErrNotReady mismatch")
	}
	if ErrStaleObservation != installauthority.ErrStaleObservation {
		t.Fatalf("ErrStaleObservation mismatch")
	}
	if ErrInvalidObservation != installauthority.ErrInvalidObservation {
		t.Fatalf("ErrInvalidObservation mismatch")
	}
}
