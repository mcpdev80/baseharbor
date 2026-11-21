package main

import (
	"errors"
	"strings"
	"testing"
)

// 0428-207 regression: coreDefaultNetworkBindingMissingOwnerMessage (the guard's
// refusal renderer at cmd/baha/core_network_binding.go, called from
// inspectCoreDefaultNetworkBinding) must name BOTH halves of the missing
// precondition (protected Core runtime inventory OR shared backend owner) and
// how to satisfy it, instead of the unactionable
// "existing native network lacks a protected Core or shared backend owner".
// This is the read-only round's corrected production site, NOT
// application_provider_impact.go:63.
func Test0428207CoreNetworkBindingGuardNamesMissingPreconditions(t *testing.T) {
	// Missing inventory (os.ErrNotExist): the App-Edit baseline where the
	// isolated Core has never materialized its runtime inventory.
	msg := coreDefaultNetworkBindingMissingOwnerMessage(errors.New("lstat ... : no such file or directory")).Error()
	// (a) names the missing protected Core runtime inventory precondition ...
	if !strings.Contains(msg, "protected Core") {
		t.Fatalf("0428-207: message must name the missing protected Core inventory, got %q", msg)
	}
	// ... AND the shared backend owner precondition.
	if !strings.Contains(msg, "shared backend owner") {
		t.Fatalf("0428-207: message must name the missing shared backend owner, got %q", msg)
	}
	// (b) names the check that verifies it (canonical protected owner shape).
	if !strings.Contains(msg, "canonical") {
		t.Fatalf("0428-207: message must name the verification check (canonical protected owner shape), got %q", msg)
	}
	// (c) names the user's next step (materialize the Core, then retry).
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "baha app start") && !strings.Contains(lower, "retry") {
		t.Fatalf("0428-207: message must name the user's next step, got %q", msg)
	}
	// The old unactionable string is replaced; only the message changed, never
	// the fail-closed refusal.
	if strings.Contains(msg, "lacks a protected Core or shared backend owner") {
		t.Fatalf("0428-207: old unactionable message must be replaced, got %q", msg)
	}
	t.Log("0428-207 missing-inventory guard names missing protected Core + shared backend owner, canonical check, and next step")
}

// 0428-207 noncanonical-shape variant: the native control-plane definition
// differs from its canonical protected owner shape (or the shared backend owner
// is absent). It must name BOTH preconditions too.
func Test0428207CoreNetworkBindingGuardNoncanonicalShape(t *testing.T) {
	msg := coreDefaultNetworkBindingMissingOwnerMessage(errors.New("native control-plane definition differs from its canonical owner shape")).Error()
	if !strings.Contains(msg, "protected Core") {
		t.Fatalf("0428-207: noncanonical message must name the missing protected Core, got %q", msg)
	}
	if !strings.Contains(msg, "shared backend owner") {
		t.Fatalf("0428-207: noncanonical message must name the missing shared backend owner, got %q", msg)
	}
	if !strings.Contains(msg, "canonical") {
		t.Fatalf("0428-207: noncanonical message must name the canonical protected owner check, got %q", msg)
	}
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "baha app start") && !strings.Contains(lower, "retry") {
		t.Fatalf("0428-207: noncanonical message must name the user's next step, got %q", msg)
	}
	t.Log("0428-207 noncanonical-shape guard names both preconditions, canonical check, and next step")
}

// 0428-207 fail-closed boundary: the guard refuses; it never silently returns
// (false, nil) when a native network has no verifiable protected owner. This
// unit test asserts the message content; what it does NOT prove is stated in
// the akte: the guard's real entry (len(actual) != 0, a native default network
// present) requires a runtime daemon, i.e. the baha-docker lifecycle proof, and
// this unit test only pins the message wording that the production guard returns.
func Test0428207CoreNetworkBindingGuardFailClosedNeverSilent(t *testing.T) {
	missingErr := coreDefaultNetworkBindingMissingOwnerMessage(errors.New("missing"))
	if missingErr == nil {
		t.Fatalf("0428-207: missing-owner guard must never be nil")
	}
	missing := missingErr.Error()
	if strings.Contains(missing, "lacks a protected Core or shared backend owner") {
		t.Fatalf("0428-207: old unactionable message must be replaced, got %q", missing)
	}
}
