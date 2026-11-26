package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// neverDeliveredProofDirFor resolves the never-delivered/<applicationID> directory
// for an identity via the deployment API. It is a test helper; production code
// uses the same path function in app_runtime_retirement.go.
func neverDeliveredProofDirFor(t *testing.T, id deployment.DeploymentIdentity) string {
	t.Helper()
	dir, err := deployment.NeverDeliveredProofDir(id.Target, id.ApplicationID)
	if err != nil {
		t.Fatalf("never-delivered proof dir for %s: %v", id.ApplicationID, err)
	}
	return dir
}

// neverDeliveredRetireFixture builds an application that is initialized (identity
// in the .baseharbor/apps store, valid one-shot proof present) but has NEVER had
// a deployment record persisted on disk: the deployment.json was lost after the
// identity was created. This is the exact never-delivered state that
// verifyOwnershipWhenNoDeploymentRecord recognises and retireRuntimePreservingData
// must retire on the clean path. The fixture drives the REGISTERED command route
// (app down --yes, stop mode => fullReset=false => retireRuntimePreservingData)
// directly on the destroy execution, bypassing the
// finishUninitializedRepositoryDestroy no-op gate, which is the production path
// that the card scopes to.
func neverDeliveredRetireFixture(t *testing.T) (resolvedApplication, string, string) {
	t.Helper()
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithHTTPExposure(m, "demo-app", "demo-app", 8080, "https")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	// Initialize the identity. This writes the application store entry AND the
	// one-shot never-delivered proof.
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for init: %v", err)
	}
	proofFile := filepath.Join(neverDeliveredProofDirFor(t, resolved.DeploymentIdentity), resolved.DeploymentIdentity.Environment+".json")
	if _, statErr := os.Stat(proofFile); statErr != nil {
		t.Fatalf("expected never-delivered proof to exist after init: %v", statErr)
	}
	// No deployment record was ever persisted: this identity was initialized but
	// never delivered, so the proof must still be present.
	if _, statErr := os.Stat(proofFile); statErr != nil {
		t.Fatalf("proof must exist for the never-delivered fixture: %v", statErr)
	}
	return resolved, neverDeliveredProofDirFor(t, resolved.DeploymentIdentity), proofFile
}

// TestW341NeverDeliveredRetireConsumesProof is the permanent regression for
// t-20261126 (hunt H-V46, BF-46-2, card 0428-326). It proves the fix: an app
// initialized but never delivered retires on the never-delivered path and
// CONSUMES the one-shot proof (ConsumeNeverDeliveredProof) and removes the
// now-empty never-delivered/<applicationID> directory, so the O_EXCL collision
// between guided adoption and lifecycle destroy cannot occur.
func TestW341NeverDeliveredRetireConsumesProof(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
	resolved, proofDir, proofFile := neverDeliveredRetireFixture(t)

	execution, err := newApplicationDestroyExecution(ctx, resolved.Store, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("destroy execution for never-delivered fixture: %v", err)
	}
	if execution.confirmed {
		t.Fatal("never-delivered retire must require --yes confirmation")
	}
	execution.confirmed = true
	var out bytes.Buffer
	if err := execution.retireRuntimePreservingData(ctx, &out); err != nil {
		t.Fatalf("never-delivered retire must return rc=0, got: %v\noutput:\n%s", err, out.String())
	}

	// The one-shot proof file and its directory are gone.
	if _, statErr := os.Stat(proofFile); !os.IsNotExist(statErr) {
		t.Fatalf("never-delivered retire must consume the proof file, still present: %v", statErr)
	}
	if _, statErr := os.Stat(proofDir); !os.IsNotExist(statErr) {
		t.Fatalf("never-delivered retire must remove the now-empty proof directory, still present: %v", statErr)
	}
}

// TestW341NeverDeliveredRetireFailsClosedOnMismatchedProof proves the negative:
// consumption only removes a proof that this lane actually verified as consumed
// by the same identity. A mismatched/foreign proof written at this identity's
// path (wrong application content) must keep the retire path fail-closed: the
// verify gate never trusted it, so the retire never consumes it.
func TestW341NeverDeliveredRetireFailsClosedOnMismatchedProof(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, applicationStopModeKey{}, true)
	resolved, proofDir, proofFile := neverDeliveredRetireFixture(t)

	// Overwrite THIS identity's proof with a mismatched payload (foreign app
	// content at this identity's proof path). The verify gate must fail closed.
	mismatched, derr := deployment.NewDeploymentIdentity(
		resolved.DeploymentIdentity.Target,
		"00000000-0000-4000-8000-000000000000",
		"foreign-app",
		resolved.DeploymentIdentity.Environment,
	)
	if derr != nil {
		t.Fatalf("build mismatched identity: %v", derr)
	}
	if statErr := os.MkdirAll(proofDir, 0o700); statErr != nil {
		t.Fatalf("mkdir never-delivered dir: %v", statErr)
	}
	mismatchData, _ := json.Marshal(deployment.NeverDeliveredProof{
		Version: 1, ApplicationID: mismatched.ApplicationID, Application: mismatched.Application,
		Environment: resolved.DeploymentIdentity.Environment, Target: mismatched.Target,
	})
	if statErr := os.WriteFile(proofFile, append(mismatchData, '\n'), 0o600); statErr != nil {
		t.Fatalf("seed mismatched proof: %v", statErr)
	}
	if err := deployment.VerifyNeverDeliveredProof(resolved.DeploymentIdentity); err == nil {
		t.Fatal("verify gate should fail closed on a mismatched proof at this identity's path")
	}

	execution, err := newApplicationDestroyExecution(ctx, resolved.Store, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("destroy execution for never-delivered fixture: %v", err)
	}
	execution.confirmed = true
	var out bytes.Buffer
	if derr := execution.retireRuntimePreservingData(ctx, &out); derr == nil {
		t.Fatalf("destroy against a mismatched proof must fail closed, got rc=0\n%s", out.String())
	}

	// The mismatched proof is still present and unpruned: the retire never
	// consumed what the verify gate never trusted.
	if _, statErr := os.Stat(proofFile); statErr != nil {
		t.Fatalf("mismatched proof must be left intact by a failed retire: %v", statErr)
	}
}
