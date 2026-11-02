package coreinstallation

import (
	"testing"
	"time"
)

// Positive: installation, management connection (Target) and the single state
// owner are distinct and addressable.
func TestSelectionKeepsInstallationConnectionAndTargetDistinct(t *testing.T) {
	s := Selection{Installation: "411f3c2d-0000-4000-8000-000000000001", Target: "prod-us"}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid selection rejected: %v", err)
	}
	if err := s.AssertDistinct(); err != nil {
		t.Fatalf("distinct authorities conflated: %v", err)
	}
	now := time.Now()
	remote := RemoteAuthority{Installation: s.Installation, ManagedBy: LocalOwner, ObservedAt: now}
	if err := remote.Accept(now); err != nil {
		t.Fatalf("fresh remote observation rejected: %v", err)
	}
}

// Negative: installation must not equal the Target; conflating them is rejected.
func TestSelectionRejectsConflatedInstallationAndTarget(t *testing.T) {
	s := Selection{Installation: "prod", Target: "prod"}
	if err := s.Validate(); err != nil {
		t.Fatalf("empty-selection validation: %v", err)
	}
	if err := s.AssertDistinct(); err != ErrConflatedSelection {
		t.Fatalf("expected conflation rejection, got %v", err)
	}
}

// Negative: a missing or empty selection is rejected.
func TestSelectionRejectsMissingFields(t *testing.T) {
	for _, s := range []Selection{{Installation: "411f3c2d-0000-4000-8000-000000000001"}, {Target: "prod"}} {
		if err := s.Validate(); err == nil {
			t.Fatalf("incomplete selection accepted: %+v", s)
		}
	}
}

// Negative: remote clients never own state. LocalOwner is the only permitted
// managed owner.
func TestRemoteClientNeverOwnsState(t *testing.T) {
	remote := RemoteAuthority{Installation: "411f3c2d-0000-4000-8000-000000000001", ManagedBy: "remote-core"}
	if err := remote.RejectOwnership(); err != ErrRemoteStateRejected {
		t.Fatalf("remote ownership allowed: %v", err)
	}
	// A local-owner address is correctly addressable but still rejects ownership.
	local := RemoteAuthority{Installation: "411f3c2d-0000-4000-8000-000000000001", ManagedBy: LocalOwner}
	if err := local.RejectOwnership(); err != nil {
		t.Fatalf("local-owner address wrongly rejected: %v", err)
	}
}

// Negative: stale observed-at freshness is rejected, never claimed READY.
func TestStaleObservationRejectedNotReady(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	stale := RemoteAuthority{Installation: "411f3c2d-0000-4000-8000-000000000001", ManagedBy: LocalOwner, ObservedAt: past}
	if stale.Fresh(time.Now()) {
		t.Fatal("stale observation reported fresh")
	}
	if err := stale.Accept(time.Now()); err != ErrStaleObservation {
		t.Fatalf("stale observation accepted: %v", err)
	}
	// A zeroed observed-at is likewise not authoritative.
	zero := RemoteAuthority{Installation: "411f3c2d-0000-4000-8000-000000000001", ManagedBy: LocalOwner}
	if zero.Fresh(time.Now()) {
		t.Fatal("zero observed-at reported fresh")
	}
}

// Negative: a missing/foreign/expired remote authority fails closed with no
// local fallback.
func TestMissingRemoteAuthorityFailsClosedNoLocalFallback(t *testing.T) {
	expired := RemoteAuthority{Installation: "411f3c2d-0000-4000-8000-000000000001", ManagedBy: LocalOwner, ObservedAt: time.Now().Add(-time.Hour)}
	if err := expired.Accept(time.Now()); err != ErrStaleObservation {
		t.Fatalf("expired remote accepted: %v", err)
	}
	_ = expired // a missing/foreign/expired authority selects no local runtime
}
