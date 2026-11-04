package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestW120RefusedApplyWritesNothingLeavesDestroyableState is the red-first
// regression for t-20261104-r27. A candidate apply refused by the consent gate
// (wrong --confirm-candidate digest) must leave the prior deployment record
// UNCHANGED (rollback-free), so a later `app destroy` can proceed.
//
// Before the fix the refused apply left a partial applying deployment.json whose
// runtime has no owner; destroy failed with ErrRuntimeNotApplied.
func TestW120RefusedApplyWritesNothingLeavesDestroyableState(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  demo-app:\n    build: .\n    ports:\n      - '8080:8080'\n")
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

	// Wrong confirm digest must fail closed at the candidate choke point.
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	wrongCtx := withCandidateConsentDigest(context.Background(), wrong)
	if _, err := recordDeploymentBeforeMutation(wrongCtx, resolved, "applying", wrong); err == nil {
		t.Fatal("wrong confirm digest accepted; candidate must fail closed")
	}

	// (c) The refused apply must have left the prior record UNCHANGED: still
	// "configured", no consent, no success state.
	after, err := deployment.LoadDeploymentRecord(pending.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if after.Observed.State != "configured" {
		t.Fatalf("refused apply mutated state to %q; must leave prior record intact", after.Observed.State)
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

	// (a) A destroy of the untouched record must be able to resolve the Core
	// runtime files path (no dangling owner). We assert destroy can resolve the
	// application and reach the destroy execution without the partial-state
	// error; the full Docker teardown is covered by the acceptance gate.
	_, derr := resolveApplication(context.Background(), application.DefaultStore(), nil, "destroy")
	if derr != nil {
		t.Fatalf("destroy cannot re-resolve after refused apply: %v", derr)
	}
	t.Log("refused apply left prior record intact; destroy can re-resolve")
}

// TestW120ConsentedApplyPassesChokePointAfterRefusal proves (b): after a refused
// apply, an apply with the CORRECT confirm digest passes the candidate choke
// point and writes a validated applying record (the first step to READY).
func TestW120ConsentedApplyPassesChokePointAfterRefusal(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), "services:\n  demo-app:\n    build: .\n    ports:\n      - '8080:8080'\n")
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
