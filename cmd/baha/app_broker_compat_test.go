package main

import "testing"

// TestVerifyRuntimeBrokerBuildIdentityCommitMismatch guards the commit-provenance
// fix: verifyRuntimeBrokerBuildIdentity fails closed when the CLI and the
// runtime report concrete, differing commits at the same semver. Regression for
// 0428-271, which discarded both commit SHAs and let a wrong-commit broker pass
// readiness on a matching version.
func TestVerifyRuntimeBrokerBuildIdentityCommitMismatch(t *testing.T) {
	oldVersion, oldCommit := version, commit
	t.Cleanup(func() {
		version, commit = oldVersion, oldCommit
	})

	version = "0.4.15"
	commit = "cli-commit-123"

	// Same version, same commit: still valid.
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", "cli-commit-123"); err != nil {
		t.Fatalf("matching runtime commit rejected: %v", err)
	}

	// Same version, different concrete commit: must fail closed.
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", "other-commit-999"); err == nil {
		t.Fatal("matching semver with a different concrete commit must be rejected")
	}

	// Same version, but the runtime reports no commit: keep the dev/edge-style
	// leniency (a single empty commit never fails the probe).
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", ""); err != nil {
		t.Fatalf("empty runtime commit must not fail readiness: %v", err)
	}
}

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
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", "abc123"); err != nil {
		t.Fatalf("matching runtime commit must not fail: %v", err)
	}
	if err := verifyRuntimeBrokerBuildIdentity("", ""); err == nil {
		t.Fatal("missing runtime build identity was accepted")
	}

	version = "dev"
	commit = "none"
	// A development CLI still reports a concrete commit ("none"); with the
	// fail-closed commit check that means it must only accept a broker at the
	// same commit, not an arbitrary one. The truly-empty commit is the
	// dev/edge-style leniency, not this "none" case.
	if err := verifyRuntimeBrokerBuildIdentity("edge", "none"); err != nil {
		t.Fatalf("dev CLI should accept an edge broker at the same commit: %v", err)
	}
	if err := verifyRuntimeBrokerBuildIdentity("edge", "some-edge-commit"); err == nil {
		t.Fatal("dev CLI must reject an edge broker at a different commit")
	}
}
