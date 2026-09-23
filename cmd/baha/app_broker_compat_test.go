package main

import "testing"

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
	if err := verifyRuntimeBrokerBuildIdentity("0.4.14", "abc123"); err == nil {
		t.Fatal("mismatched runtime version was accepted")
	}
	if err := verifyRuntimeBrokerBuildIdentity("0.4.15", "old"); err == nil {
		t.Fatal("mismatched runtime commit was accepted")
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
