package installauthority

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeIssuer is a test double for the Issuer decision surface.
type fakeIssuer struct {
	ready  bool
	revErr error
}

func (f *fakeIssuer) Status(ctx context.Context) (IssuerStatus, error) {
	return IssuerStatus{Ready: f.ready}, nil
}

func (f *fakeIssuer) Revoke(ctx context.Context, serial string) error {
	return f.revErr
}

func TestFreshWithinBound(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	obs := CertificateStatus{IssuedCertificate: IssuedCertificate{ExpiresAt: now.Add(time.Minute), Serial: "ABC"}}
	if err := obs.Fresh(now); err != nil {
		t.Fatalf("fresh observation rejected: %v", err)
	}
}

func TestFreshZeroRejected(t *testing.T) {
	obs := CertificateStatus{IssuedCertificate: IssuedCertificate{ExpiresAt: time.Time{}}}
	if err := obs.Fresh(time.Now()); err != ErrInvalidObservation {
		t.Fatalf("zero expiration accepted, got %v", err)
	}
}

func TestFreshlyObservedConnectionValid(t *testing.T) {
	iss := &fakeIssuer{ready: true, revErr: nil}
	obs := CertificateStatus{Identity: "ref", IssuedCertificate: IssuedCertificate{Serial: "ABC", ExpiresAt: time.Now().Add(time.Hour)}}
	if err := FreshlyObservedConnection(context.Background(), iss, obs, "ref"); err != nil {
		t.Fatalf("valid connection rejected: %v", err)
	}
}

func TestFreshlyObservedConnectionForeign(t *testing.T) {
	iss := &fakeIssuer{ready: true}
	obs := CertificateStatus{Identity: "ref", IssuedCertificate: IssuedCertificate{Serial: "ABC"}}
	if err := FreshlyObservedConnection(context.Background(), iss, obs, "other"); err != ErrForeignAuthority {
		t.Fatalf("foreign identity should fail closed, got %v", err)
	}
}

func TestFreshlyObservedConnectionMissingIssuer(t *testing.T) {
	obs := CertificateStatus{Identity: "ref"}
	if err := FreshlyObservedConnection(context.Background(), nil, obs, "ref"); err != ErrMissingAuthority {
		t.Fatalf("missing issuer should fail closed, got %v", err)
	}
}

func TestFreshlyObservedConnectionNotReady(t *testing.T) {
	iss := &fakeIssuer{ready: false}
	obs := CertificateStatus{Identity: "ref", IssuedCertificate: IssuedCertificate{Serial: "ABC"}}
	if err := FreshlyObservedConnection(context.Background(), iss, obs, "ref"); err != ErrNotReady {
		t.Fatalf("not-ready should fail closed, got %v", err)
	}
}

func TestFreshlyObservedConnectionRevoked(t *testing.T) {
	iss := &fakeIssuer{ready: true, revErr: errors.New("revoked")}
	exp := time.Now().Add(time.Hour)
	obs := CertificateStatus{Identity: "ref", IssuedCertificate: IssuedCertificate{Serial: "ABC", ExpiresAt: exp}}
	if err := FreshlyObservedConnection(context.Background(), iss, obs, "ref"); err != ErrForeignAuthority {
		t.Fatalf("revoked should fail closed, got %v", err)
	}
}
