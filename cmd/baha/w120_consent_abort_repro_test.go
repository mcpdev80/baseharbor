package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestW120RefusedApplyWritesNothingLeavesDestroyableState is the red-first
// regression for t-20261104-r27. It drives the REAL candidate choke point
// (recordDeploymentBeforeMutation), not merely resolveApplication(..., "destroy"),
// and proves the rollback-free invariant that the reported bug violated.
//
// Before the fix the refused apply left a partial applying deployment.json whose
// runtime had no owner; `app destroy` then failed with ErrRuntimeNotApplied.
//
// Red on the reported-bug state: the assertion after.Observed.State == "configured"
// fails (the record was left "applying" with a dangling owner). Green on the fix:
// the choke point aborts before any state write, so the prior "configured" record
// survives untouched.
func TestW120RefusedApplyWritesNothingLeavesDestroyableState(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := recordPendingDeployment(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}

	// Wrong confirm digest must fail closed at the candidate choke point and
	// write NO deployment state.
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	wrongCtx := withCandidateConsentDigest(context.Background(), wrong)
	if _, err := recordDeploymentBeforeMutation(wrongCtx, resolved, "applying", wrong); err == nil {
		t.Fatal("wrong confirm digest accepted; candidate must fail closed")
	}

	// (a) The refused apply must have left the prior record UNCHANGED: still
	// "configured", no consent, no success state, no previous-intent projection.
	// This is the exact reported-bug assertion: on the buggy state the record was
	// left "applying" with a dangling runtime owner.
	after, err := deployment.LoadDeploymentRecord(pending.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if after.Observed.State != "configured" {
		t.Fatalf("refused apply mutated state to %q; must leave prior record intact (rollback-free)", after.Observed.State)
	}
	if after.Observed.Ready {
		t.Fatalf("refused apply recorded a ready state")
	}
	if after.Consent != nil {
		t.Fatalf("refused apply wrote a consent: %+v", after.Consent)
	}
	if after.Applied.GeneratedState["previous_intent"] != "" {
		t.Fatalf("refused apply left a previous-intent projection: %q", after.Applied.GeneratedState["previous_intent"])
	}

	// (b) The refused apply must not have left a partial applying record that
	// blocks the destroy lifecycle. The destroyable precondition is that the
	// deployment re-resolves on the destroy command (no dangling owner) and the
	// persisted record is still "configured" (never mutated to "applying").
	_, derr := resolveApplication(context.Background(), application.DefaultStore(), nil, "destroy")
	if derr != nil {
		t.Fatalf("destroy cannot re-resolve after refused apply: %v", derr)
	}
	if after.Observed.State == "applying" {
		t.Fatalf("refused apply left a partial applying record; destroy would fail with ErrRuntimeNotApplied (state=%q)", after.Observed.State)
	}

	t.Log("refused apply left prior record intact (rollback-free); no dangling runtime owner; destroy can re-resolve")
}

// TestW120ConsentedApplyPassesChokePointAfterRefusal proves the follow-up: after
// a refused apply, an apply with the CORRECT confirm digest passes the candidate
// choke point and writes a validated applying record (the first step to READY),
// without a manual rollback or cleanup in between.
func TestW120ConsentedApplyPassesChokePointAfterRefusal(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recordPendingDeployment(context.Background(), resolved); err != nil {
		t.Fatal(err)
	}

	// Wrong digest first -> refused, writes nothing.
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := recordDeploymentBeforeMutation(withCandidateConsentDigest(context.Background(), wrong), resolved, "applying", wrong); err == nil {
		t.Fatal("wrong confirm digest accepted; candidate must fail closed")
	}

	// Correct digest: compute the canonical confirm digest the real apply binds.
	correct := confirmDigestForResolved(t, resolved)
	pending, err := recordDeploymentBeforeMutation(withCandidateConsentDigest(context.Background(), correct), resolved, "applying", correct)
	if err != nil {
		t.Fatalf("correct confirm digest rejected at choke point: %v", err)
	}
	if pending.Consent == nil {
		t.Fatal("correct confirm produced no consent")
	}
	if pending.Observed.State != "applying" {
		t.Fatalf("consented apply did not record applying state: %q", pending.Observed.State)
	}
	t.Log("consented apply passes choke point and writes applying record with consent")
}
