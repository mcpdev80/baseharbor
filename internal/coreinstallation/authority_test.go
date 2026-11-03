package coreinstallation

import (
	"context"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
)

// fakeIssuer is a test double for the coreinstallation.Issuer decision surface.
// It intentionally does NOT import serviceaccess so the test package stays
// acyclic (serviceaccess imports coreinstallation for the lifecycle lock).

// liveFake returns a fake issuer that is ready and rejects nothing, used for
// positive and identity/foreign/freshness cases.
type fakeIssuer struct {
	ready  bool
	serial string
	revErr error
}

func (f *fakeIssuer) Status(ctx context.Context) (installauthority.IssuerStatus, error) {
	return installauthority.IssuerStatus{Ready: f.ready}, nil
}

func (f *fakeIssuer) Revoke(ctx context.Context, serial string) error {
	return f.revErr
}

func newFakeIssuer() *fakeIssuer {
	return &fakeIssuer{ready: true, serial: "ABC123", revErr: nil}
}

func newRevokingIssuer() *fakeIssuer {
	return &fakeIssuer{ready: true, serial: "ABC123", revErr: context.Canceled}
}

func liveObserved(ref string) installauthority.CertificateStatus {
	return installauthority.CertificateStatus{
		Identity:            ref,
		IssuedCertificate:   installauthority.IssuedCertificate{IssuerReference: ref, Serial: "ABC123", ExpiresAt: time.Now().Add(time.Hour)},
	}
}

// --- Finding 1: three distinct roles as real, interchanged authorities ---

func TestThreeRolesAreDistinctInstallationConnectionAndTarget(t *testing.T) {
	s := Selection{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us"}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}
	if err := s.AssertDistinct(); err != nil {
		t.Fatalf("distinct authorities conflated: %v", err)
	}

	remote := ObservedState{Installation: s.Installation, Target: s.Target, ObservedAt: time.Now()}
	if err := remote.RejectOwnership(); err != ErrRemoteStateRejected {
		t.Fatalf("remote ownership should fail closed: %v", err)
	}
}

func TestConflatingInstallationAndTargetIsRejected(t *testing.T) {
	s := Selection{Installation: "prod", Target: "prod"}
	if err := s.AssertDistinct(); err != ErrConflatedSelection {
		t.Fatalf("expected conflation rejection, got %v", err)
	}
}

func TestIncompleteSelectionRejected(t *testing.T) {
	for _, s := range []Selection{
		{Installation: "411f3c2d-0000-4000-8000-000000000001"},
		{Target: "prod"},
		{},
	} {
		if err := s.Validate(); err == nil {
			t.Fatalf("incomplete selection accepted: %+v", s)
		}
	}
}

// --- Finding 3: freshness = now <= Issuer.ExpiresAt, zero/future fail-closed ---

func TestFreshWithinExpiresAt(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	o := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: now.Add(-30 * time.Minute)}
	if err := o.Fresh(now); err != nil {
		t.Fatalf("fresh observation rejected: %v", err)
	}
}

func TestFreshExpiredPastExpiresAt(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	obs := installauthority.CertificateStatus{
		Identity:          "ref",
		IssuedCertificate: installauthority.IssuedCertificate{ExpiresAt: now.Add(-2 * time.Hour)},
	}
	if err := obs.Fresh(now); err != installauthority.ErrStaleObservation {
		t.Fatalf("expired observation accepted, got %v", err)
	}
}

func TestFreshZeroRejected(t *testing.T) {
	now := time.Now().UTC()
	o := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us"}
	if err := o.Fresh(now); err != ErrInvalidObservation {
		t.Fatalf("zero observation accepted, got %v", err)
	}
}

func TestFreshFutureRejected(t *testing.T) {
	now := time.Now().UTC()
	o := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: now.Add(time.Hour)}
	if err := o.Fresh(now); err != ErrFutureObservation {
		t.Fatalf("future observation accepted, got %v", err)
	}
}

// --- Positive path (finding 1/2): real selection over the leaf boundary ---

func TestAuthoritativeConnectionValidRealSelection(t *testing.T) {
	iss := newFakeIssuer()
	obs := liveObserved("ref")
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, "ref"); err != nil {
		t.Fatalf("valid authoritative connection rejected: %v", err)
	}
}

func TestVerifyConnectionIdentityValidRealSelection(t *testing.T) {
	iss := newFakeIssuer()
	obs := liveObserved("ref")
	if err := VerifyConnectionIdentity(context.Background(), iss, obs, "ref"); err != nil {
		t.Fatalf("valid connection identity rejected: %v", err)
	}
}

// --- Foreign/revoked path ---

func TestAuthoritativeConnectionForeignIdentityFailsClosed(t *testing.T) {
	iss := newFakeIssuer()
	obs := liveObserved("ref")
	if err := remoteTest(t, iss, obs, "foreign-identity"); err != ErrForeignAuthority {
		t.Fatalf("foreign identity should fail closed, got %v", err)
	}
}

func TestAuthoritativeConnectionRevokedFailsClosed(t *testing.T) {
	iss := newRevokingIssuer()
	obs := liveObserved("ref")
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, "ref"); err != ErrForeignAuthority {
		t.Fatalf("revoked connection should fail closed, got %v", err)
	}
}

// --- Missing authority (fails closed) ---

func TestAuthoritativeConnectionMissingIssuerFailsClosed(t *testing.T) {
	obs := liveObserved("ref")
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), nil, obs, "x"); err != ErrMissingAuthority {
		t.Fatalf("missing authority should fail closed, got %v", err)
	}
}

// --- Freshness sentinels ---

func TestAuthoritativeConnectionZeroExpiryFailsClosed(t *testing.T) {
	iss := newFakeIssuer()
	obs := installauthority.CertificateStatus{
		Identity:          "ref",
		IssuedCertificate: installauthority.IssuedCertificate{}, // zero ExpiresAt
	}
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, "ref"); err != ErrInvalidObservation {
		t.Fatalf("zero expiration should fail closed, got %v", err)
	}
}

func TestAuthoritativeConnectionNotReadyFailsClosed(t *testing.T) {
	iss := &fakeIssuer{ready: false}
	obs := liveObserved("ref")
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, "ref"); err != ErrNotReady {
		t.Fatalf("not-ready connection should fail closed, got %v", err)
	}
}

// --- Genuine installation gate ---

func TestStateIsGenuineInstallationAcceptsAndRejectsForeign(t *testing.T) {
	if err := AssertStateIsGenuineInstallation(State{ID: "411f3c2d-0000-4000-8000-000000000001", Owner: "baseharbor"}); err != nil {
		t.Fatalf("genuine installation rejected: %v", err)
	}
	if err := AssertStateIsGenuineInstallation(State{ID: "x", Owner: "other"}); err != ErrForeignAuthority {
		t.Fatalf("foreign installation accepted: %v", err)
	}
}

// --- Leaf boundary: FreshlyObservedConnection directly (foreign + revoked) ---

func TestFreshlyObservedConnectionForeignIdentity(t *testing.T) {
	obs := installauthority.CertificateStatus{Identity: "foreign"}
	if err := installauthority.FreshlyObservedConnection(context.Background(), nil, obs, "ref"); err != installauthority.ErrForeignAuthority {
		t.Fatalf("foreign identity should fail closed, got %v", err)
	}
}

func TestFreshlyObservedConnectionMissingIssuer(t *testing.T) {
	obs := installauthority.CertificateStatus{Identity: "ref"}
	if err := installauthority.FreshlyObservedConnection(context.Background(), nil, obs, "ref"); err != installauthority.ErrMissingAuthority {
		t.Fatalf("missing issuer should fail closed, got %v", err)
	}
}

func remoteTest(t *testing.T, iss Issuer, obs installauthority.CertificateStatus, authoritativeConnection string) error {
	t.Helper()
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	return remote.AuthoritativeConnection(context.Background(), iss, obs, authoritativeConnection)
}
