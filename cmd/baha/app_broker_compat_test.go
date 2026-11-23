package main

import (
	"strings"
	"testing"
)

func TestVerifyRuntimeBrokerBuildIdentity(t *testing.T) {
	oldVersion, oldCommit := version, commit
	t.Cleanup(func() {
		version, commit = oldVersion, oldCommit
	})

	version = "0.4.15"
	commit = "abc123"
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", "abc123"); err != nil {
		t.Fatalf("matching runtime identity rejected: %v", err)
	}
	if err := verifyRuntimeBrokerBuildIdentity("v0.4.15", "abc123"); err != nil {
		t.Fatalf("v-prefixed matching runtime identity rejected: %v", err)
	}
	if err := verifyRuntimeBrokerBuildIdentity("0.4.14", "abc123"); err == nil {
		t.Fatal("mismatched runtime version was accepted")
	}
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", "old"); err != nil {
		t.Fatalf("commit provenance mismatch must not define compatibility: %v", err)
	}
	if err := verifyRuntimeBrokerBuildIdentity("", ""); err == nil {
		t.Fatal("missing runtime build identity was accepted")
	}

	version = "dev"
	commit = "none"
	if err := verifyRuntimeBrokerBuildIdentity("edge", "some-edge-commit"); err != nil {
		t.Fatalf("development CLI should accept edge runtime identity: %v", err)
	}
}

// TestVerifyRuntimeBrokerBuildIdentityMismatchReportsActualVersion guards the
// mismatch diagnostic: when the runtime version does not match the CLI's
// expected version, the error must report the ACTUAL runtime version observed,
// never the CLI's expected version duplicated as if it were the runtime's.
// Regression for a fix-loud that echoed expectedVersion in the "got" slot.
func TestVerifyRuntimeBrokerBuildIdentityMismatchReportsActualVersion(t *testing.T) {
	oldVersion, oldCommit := version, commit
	t.Cleanup(func() {
		version, commit = oldVersion, oldCommit
	})
	version = "0.4.15"
	commit = "abc123"

	const observedRuntimeVersion = "0.4.14"
	err := verifyRuntimeBrokerBuildIdentity(observedRuntimeVersion, "different-commit")
	if err == nil {
		t.Fatal("mismatched runtime version must be rejected")
	}
	msg := err.Error()
	// The true observed runtime value must appear in the diagnostic...
	if !strings.Contains(msg, observedRuntimeVersion) {
		t.Fatalf("mismatch diagnostic must report the actual runtime version %q: %q", observedRuntimeVersion, msg)
	}
	// ...and the CLI's expected version must also appear, distinctly.
	if !strings.Contains(msg, "0.4.15") {
		t.Fatalf("mismatch diagnostic must also name the CLI's expected version: %q", msg)
	}
	// The runtime version must appear exactly once (not duplicated from the
	// expected value).
	if strings.Count(msg, observedRuntimeVersion) != 1 {
		t.Fatalf("diagnostic must report the runtime version exactly once (not duplicated): %q", msg)
	}
}
