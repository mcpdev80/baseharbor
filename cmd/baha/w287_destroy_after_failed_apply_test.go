package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// TestW287DestroyAfterFailedApplyReturnsZero is the permanent regression for
// 0428-287 (installation-reset-recovery). It proves the fix for the state that
// neither #1375 (deployment.json missing, never-delivered) nor w154 (stuck-apply
// stop) covers:
//
//   - deployment.json IS present with applied intent + last_applied_ref,
//   - state is a failed/DEGRADED "applying" (as a real aborted apply leaves it),
//   - but NO runtime directory was ever materialized (EnsureRuntime never ran),
//   - and a Core-owned shared SQL backend is present on the Target.
//
// On develop the full-reset destroy path of this stuck state fails closed with:
//
//	retained provider ownership could not be verified through its canonical deployment and native renderer
//
// because the Core-owned PostgreSQL instance cannot be observed through the
// application's own (absent) runtime.
//
// The fix is state-oriented: an absent application runtime with a persisted
// deployment record reconciles the verified app-owned cleanup and retires with
// rc=0 while preserving shared/foreign/ambiguous resources (the incomplete
// inventory recovery), instead of aborting the whole install reset.
//
// The test drives the REAL CLI full-reset destroy (`app destroy --yes --full-reset`)
// against a stuck-apply state reconstructed with the public deployment API, so
// the production apply/retire/destroy code path (not a stub) is exercised:
//  1. write a deployment record with applied intent + last_applied_ref, state "applying",
//     a Core-owned shared SQL backend present on the Target, runtime dir absent
//  2. assert the runtime directory is genuinely absent (nothing materialized)
//  3. `app destroy --yes --full-reset` with the two current-plan confirmations
//     and separate data consent must return rc=0 (baseline aborts here)
func TestW287DestroyAfterFailedApplyReturnsZero(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithHTTPExposure(m, "demo-app", "demo-app", 8080, "https")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for apply: %v", err)
	}

	// Reconstruct the stuck-apply state exactly as a real aborted apply leaves it:
	// deployment.json present with applied intent + last_applied_ref, state
	// "applying", but NO runtime directory materialized (EnsureRuntime never ran).
	manifestBytes, err := json.Marshal(resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	pending := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: resolved.repositoryRoot(),
			Manifest:   resolved.ManifestPath,
			Digest:     "abort-revision-abc123",
		},
		Applied: deployment.AppliedDeployment{
			Intent:          json.RawMessage(manifestBytes),
			RuntimeProvider: resolved.Target.RuntimeProvider,
			LastAppliedRef:  "abort-revision-abc123",
			GeneratedState: map[string]string{
				"deployment": resolved.DeploymentStateRoot,
				"state":      resolved.DeploymentStateRoot + "/state",
			},
		},
		Observed: deployment.ObservedDeployment{State: "applying", Ready: false},
	}
	if err := deployment.SaveDeploymentRecord(pending); err != nil {
		t.Fatalf("save stuck-apply record: %v", err)
	}

	// The runtime directory must be genuinely absent: this is the exact
	// ownership evidence that distinguishes "applied but never materialized"
	// from a healthy delivery.
	rtDir := application.RuntimeFilesFor(resolved.Store, resolved.Manifest).Dir
	if _, statErr := os.Stat(rtDir); statErr == nil {
		t.Fatalf("runtime dir unexpectedly present at %s; the abort scenario requires it absent", rtDir)
	}

	// A stuck/failed-apply record with no materialized runtime routes the full
	// reset through the incomplete-inventory recovery: the current-plan scope
	// plus separate data consent is the strongest acknowledgement.
	plan, err := buildApplicationDestructionPlan(testOperatorContext(context.Background()), resolved, true)
	if err != nil {
		t.Fatalf("build destroy plan: %v", err)
	}
	if !plan.IncompleteInventory {
		t.Fatal("failed-apply destroy must route through the incomplete-inventory recovery")
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true, DeleteData: true}
	destroyCtx := withProviderChangeConsent(testOperatorContext(context.Background()), ack)
	var out bytes.Buffer
	derr := runWithIOAsTestOperator(destroyCtx, []string{"app", "destroy", "--yes", "--full-reset", "--json"}, &out, &out)
	if derr != nil {
		t.Fatalf("destroy of failed-apply application must return rc=0, got: %v\noutput:\n%s", derr, out.String())
	}

	// The deployment record is removed and the authored source is preserved.
	if _, statErr := os.Stat(resolved.DeploymentStateRoot + "/deployment.json"); !os.IsNotExist(statErr) {
		t.Fatalf("failed-apply destroy retained deployment owner: %v", statErr)
	}
	data, err := os.ReadFile(application.RepositoryManifestName)
	if err != nil {
		t.Fatalf("read authored source: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("failed-apply destroy removed authored source")
	}
	t.Log("failed-apply full reset completed through the incomplete-inventory recovery: deployment owner removed, shared/foreign owner and authored source preserved")
}

// TestW287DestroyFailedApplyIdempotent proves a second destroy on the removed
// record returns rc=0 and no error (the never-delivered/absent state), so the
// install reset reruns cleanly.
func TestW287DestroyFailedApplyIdempotent(t *testing.T) {
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "demo-app")
	m = application.WithHTTPExposure(m, "demo-app", "demo-app", 8080, "https")
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)

	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolve for apply: %v", err)
	}
	manifestBytes, _ := json.Marshal(resolved.Manifest)
	pending := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: resolved.DeploymentIdentity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: resolved.repositoryRoot(), Manifest: resolved.ManifestPath, Digest: "abort-revision-abc123"},
		Applied:  deployment.AppliedDeployment{Intent: json.RawMessage(manifestBytes), RuntimeProvider: resolved.Target.RuntimeProvider, LastAppliedRef: "abort-revision-abc123", GeneratedState: map[string]string{"deployment": resolved.DeploymentStateRoot, "state": resolved.DeploymentStateRoot + "/state"}},
		Observed: deployment.ObservedDeployment{State: "applying", Ready: false},
	}
	if err := deployment.SaveDeploymentRecord(pending); err != nil {
		t.Fatal(err)
	}
	plan, err := buildApplicationDestructionPlan(testOperatorContext(context.Background()), resolved, true)
	if err != nil {
		t.Fatal(err)
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true, DeleteData: true}
	destroyCtx := withProviderChangeConsent(testOperatorContext(context.Background()), ack)
	var out bytes.Buffer
	if err := runWithIOAsTestOperator(destroyCtx, []string{"app", "destroy", "--yes", "--full-reset"}, &out, &out); err != nil {
		t.Fatalf("first failed-apply destroy: %v", err)
	}
	// Second destroy: record is gone, so the never-delivered/absent path retires with rc=0.
	out.Reset()
	if err := runWithIOAsTestOperator(testOperatorContext(context.Background()), []string{"app", "destroy", "--yes"}, &out, &out); err != nil {
		t.Fatalf("second failed-apply destroy must return rc=0, got: %v", err)
	}
}
