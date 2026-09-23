package main

import (
	"strings"
	"testing"
)

func TestVerifyRuntimeBrokerBuildIdentityRejectsMissingIdentity(t *testing.T) {
	oldVersion, oldCommit := version, commit
	defer func() { version, commit = oldVersion, oldCommit }()
	version, commit = "0.4.15", "abc123"

	err := verifyRuntimeBrokerBuildIdentity("", "")
	if err == nil || !strings.Contains(err.Error(), "build identity is missing") {
		t.Fatalf("err=%v", err)
	}
}

func TestVerifyRuntimeBrokerBuildIdentityRejectsVersionMismatch(t *testing.T) {
	oldVersion, oldCommit := version, commit
	defer func() { version, commit = oldVersion, oldCommit }()
	version, commit = "0.4.15", "none"

	err := verifyRuntimeBrokerBuildIdentity("0.4.14", "other")
	if err == nil || !strings.Contains(err.Error(), "requires runtime version 0.4.15") {
		t.Fatalf("err=%v", err)
	}
}

func TestVerifyRuntimeBrokerBuildIdentityAcceptsDevelopmentEdgePair(t *testing.T) {
	oldVersion, oldCommit := version, commit
	defer func() { version, commit = oldVersion, oldCommit }()
	version, commit = "dev", "none"

	if err := verifyRuntimeBrokerBuildIdentity("edge", ""); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRuntimeBrokerBuildIdentityRejectsCommitMismatch(t *testing.T) {
	oldVersion, oldCommit := version, commit
	defer func() { version, commit = oldVersion, oldCommit }()
	version, commit = "edge", "abc123"

	err := verifyRuntimeBrokerBuildIdentity("edge", "def456")
	if err == nil || !strings.Contains(err.Error(), "CLI commit abc123") {
		t.Fatalf("err=%v", err)
	}
}
