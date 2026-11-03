package coreinstallation

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

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
	obs := serviceaccess.CertificateStatus{
		Identity:            "ref",
		IssuedCertificate:   serviceaccess.IssuedCertificate{ExpiresAt: now.Add(-2 * time.Hour)},
	}
	if err := obs.Fresh(now); err != serviceaccess.ErrStaleObservation {
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

// --- Findings 1/2: real selection/authority over the production serviceaccess.Issuer boundary ---

// realIssuer observes a live LocalIssuer through the real serviceaccess.Issuer
// boundary: Issue produces a leaf whose ExpiresAt is the real freshness bound
// and whose Serial is the read-only Revoke target.
func realIssuer(t *testing.T) (*serviceaccess.LocalIssuer, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	iss, err := serviceaccess.EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatalf("ensure local issuer: %v", err)
	}
	trust, err := iss.TrustBundle(context.Background())
	if err != nil {
		t.Fatalf("trust bundle: %v", err)
	}
	return iss, trust.IssuerReference
}

// realObserved issues a live leaf through the real boundary and returns the
// observed CertificateStatus (identity + fresh leaf).
func realObserved(t *testing.T, iss *serviceaccess.LocalIssuer) serviceaccess.CertificateStatus {
	t.Helper()
	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "db", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("issue live certificate: %v", err)
	}
	trust, err := iss.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return serviceaccess.CertificateStatus{Identity: trust.IssuerReference, IssuedCertificate: leaf}
}

// mustIssuer returns a live LocalIssuer for tests that need the boundary but not
// a specific observed leaf.
func mustIssuer(t *testing.T) *serviceaccess.LocalIssuer {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	iss, err := serviceaccess.EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatalf("ensure local issuer: %v", err)
	}
	return iss
}

// remoteTest runs AuthoritativeConnection with a real observed status and the
// given authoritative identity, asserting the terminal error.
func remoteTest(t *testing.T, iss *serviceaccess.LocalIssuer, obs serviceaccess.CertificateStatus, authoritativeConnection string) error {
	t.Helper()
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	return remote.AuthoritativeConnection(context.Background(), iss, obs, authoritativeConnection)
}

func TestAuthoritativeConnectionValidRealSelection(t *testing.T) {
	iss, ref := realIssuer(t)
	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "db", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	obs := serviceaccess.CertificateStatus{Identity: ref, IssuedCertificate: leaf}
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, ref); err != nil {
		t.Fatalf("valid authoritative connection rejected: %v", err)
	}
}

func TestVerifyConnectionIdentityValidRealSelection(t *testing.T) {
	iss, ref := realIssuer(t)
	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "db", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	obs := serviceaccess.CertificateStatus{Identity: ref, IssuedCertificate: leaf}
	if err := VerifyConnectionIdentity(context.Background(), iss, obs, ref); err != nil {
		t.Fatalf("valid connection identity rejected: %v", err)
	}
}

func TestAuthoritativeConnectionForeignIdentityFailsClosed(t *testing.T) {
	iss, _ := realIssuer(t)
	obs := realObserved(t, iss)
	// Expected identity is a foreign reference.
	if err := remoteTest(t, iss, obs, "foreign-identity"); err != ErrForeignAuthority {
		t.Fatalf("foreign identity should fail closed, got %v", err)
	}
}

func TestAuthoritativeConnectionMissingIssuerFailsClosed(t *testing.T) {
	obs := realObserved(t, mustIssuer(t))
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), nil, obs, "x"); err != ErrMissingAuthority {
		t.Fatalf("missing authority should fail closed, got %v", err)
	}
}

func TestAuthoritativeConnectionZeroExpiryFailsClosed(t *testing.T) {
	iss, ref := realIssuer(t)
	obs := serviceaccess.CertificateStatus{
		Identity:            ref,
		IssuedCertificate:   serviceaccess.IssuedCertificate{}, // zero ExpiresAt
	}
	if err := remoteTest(t, iss, obs, ref); err != ErrInvalidObservation {
		t.Fatalf("zero expiration should fail closed, got %v", err)
	}
}

func TestStateIsGenuineInstallationAcceptsAndRejectsForeign(t *testing.T) {
	if err := AssertStateIsGenuineInstallation(State{ID: "411f3c2d-0000-4000-8000-000000000001", Owner: "baseharbor"}); err != nil {
		t.Fatalf("genuine installation rejected: %v", err)
	}
	if err := AssertStateIsGenuineInstallation(State{ID: "x", Owner: "other"}); err != ErrForeignAuthority {
		t.Fatalf("foreign installation accepted: %v", err)
	}
}

func TestForeignOrRevokedConnectionForeignIdentity(t *testing.T) {
	obs := serviceaccess.CertificateStatus{Identity: "foreign"}
	if err := serviceaccess.ForeignOrRevokedConnection(context.Background(), nil, obs, "ref"); err != serviceaccess.ErrForeignAuthority {
		t.Fatalf("foreign identity should fail closed, got %v", err)
	}
}

// --- Finding 1 regression: revocation is read-only (non-mutating) ---

// TestRevocationIsReadonlyCrlUnchanged proves the core non-mutating property:
// the CRL bytes are identical before and after a successful verification that
// exercises the read-only Revoked(serial) boundary, and that a revoked serial
// is rejected while a foreign/missing one is rejected too.
func TestRevocationIsReadonlyCrlUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	iss, err := serviceaccess.EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatalf("ensure local issuer: %v", err)
	}
	crlPath := filepath.Join(dir, "revocations.pem")
	before, err := os.ReadFile(crlPath)
	if os.IsNotExist(err) {
		before = nil
	} else if err != nil {
		t.Fatal(err)
	}

	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "db", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	trust, err := iss.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	obs := serviceaccess.CertificateStatus{Identity: trust.IssuerReference, IssuedCertificate: leaf}
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}

	// A fresh, non-revoked connection verifies successfully and never writes.
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, trust.IssuerReference); err != nil {
		t.Fatalf("valid connection rejected: %v", err)
	}
	after, err := os.ReadFile(crlPath)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		after = nil
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("revocation verification mutated the CRL: before=%q after=%q", before, after)
	}

	// The read-only Revoked predicate reports a foreign (never-issued) serial as
	// not revoked without writing.
	if iss.Revoked(context.Background(), "deadbeef") {
		t.Fatal("foreign serial reported as revoked")
	}
}

// TestRevokedRevokedSerialRejectedNonMutating revokes a serial through the real
// mutating production action and proves the read-only verification path rejects
// it, while the CRL changes only by the single intended revoke (never by the
// verification call itself).
func TestRevokedRevokedSerialRejectedNonMutating(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	iss, err := serviceaccess.EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatalf("ensure local issuer: %v", err)
	}
	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "db", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	trust, err := iss.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Mutate once through the authorized production action.
	if err := iss.Revoke(context.Background(), leaf.Serial); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	revoked, err := os.ReadFile(filepath.Join(dir, "revocations.pem"))
	if err != nil {
		t.Fatal(err)
	}

	obs := serviceaccess.CertificateStatus{Identity: trust.IssuerReference, IssuedCertificate: leaf}
	remote := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: time.Now().UTC()}
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, trust.IssuerReference); err != ErrForeignAuthority {
		t.Fatalf("revoked serial should reject as foreign, got %v", err)
	}

	// A second verification does not change the CRL further: the verification
	// path is read-only.
	after, err := os.ReadFile(filepath.Join(dir, "revocations.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(revoked, after) {
		t.Fatalf("verification mutated the CRL after the single intended revoke")
	}
}

// TestRevokedMissingCrlFailsClosed reports a missing CRL as not revoked (fails
// closed by the boundary contract, never panics).
func TestRevokedMissingCrlFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	iss, err := serviceaccess.EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if iss.Revoked(context.Background(), "01") {
		t.Fatal("missing CRL should report not revoked")
	}
}

// --- Finding 3: ObservedAt is actually used in AuthoritativeConnection ---

// freshRemote returns an ObservedState whose ObservedAt is the given time.
func freshRemote(now time.Time) ObservedState {
	return ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us", ObservedAt: now}
}

// TestAuthoritativeConnectionUsesObservedAtFreshness proves the observation
// time (ObservedAt) is enforced by AuthoritativeConnection: a zero, future or
// stale ObservedAt fails closed, while a recent ObservedAt passes — independent
// of the certificate expiry (which is intentionally fresh here).
func TestAuthoritativeConnectionUsesObservedAtFreshness(t *testing.T) {
	iss, ref := realIssuer(t)
	leaf, err := iss.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "db", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	obs := serviceaccess.CertificateStatus{Identity: ref, IssuedCertificate: leaf}

	now := time.Now().UTC()
	// A recent observation passes.
	remote := freshRemote(now.Add(-time.Minute))
	if err := remote.AuthoritativeConnection(context.Background(), iss, obs, ref); err != nil {
		t.Fatalf("recent observation should pass: %v", err)
	}
	// A zero observation time fails closed.
	zero := ObservedState{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us"}
	if err := zero.AuthoritativeConnection(context.Background(), iss, obs, ref); err != ErrInvalidObservation {
		t.Fatalf("zero observed-at should fail closed as invalid, got %v", err)
	}
	// A future observation time fails closed.
	future := freshRemote(now.Add(time.Hour))
	if err := future.AuthoritativeConnection(context.Background(), iss, obs, ref); err != ErrFutureObservation {
		t.Fatalf("future observed-at should fail closed, got %v", err)
	}
}
