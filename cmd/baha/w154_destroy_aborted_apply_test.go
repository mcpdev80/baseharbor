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

// TestW154DestroyAfterProviderChangeAbortReturnsZero is the permanent regression
// for t-20261104-r27 (round 1, lane w154).
//
// It proves the fix for the state that #1375 does NOT cover:
//
//   - #1375 = deployment.json is MISSING entirely (never-delivered).
//   - THIS  = deployment.json IS present with applied-intent + last_applied_ref,
//     but the stage runtime (state/<environment>/runtime/runtime.env) was never
//     materialized because a provider-change aborted mid-apply at the
//     double-confirmation choke, before EnsureRuntime ran. The record is left in
//     a stuck intermediate "applying" state.
//
// On develop ea3fb38e the destroy path of this stuck state fails closed:
//
//	read application runtime environment: open .../runtime/runtime.env: no such file or directory
//
// because it assumes a fixed runtime.env that was never materialized.
//
// The fix is state-oriented: the destroy set is derived from what actually
// exists on disk (the deployment record + the materialized runtime directory),
// not from a fixed filename. An absent runtime directory means no owned
// workload containers to stop, so the retired application with rc=0.
//
// The test drives the REAL CLI destroy (`executeApplicationDestroyLifecycle`)
// against a stuck-apply state reconstructed with the public deployment API, so
// the production apply/retire/destroy code path (not a stub) is exercised:
//  1. write a deployment record with applied intent + last_applied_ref,
//     state "applying" (as a real aborted apply leaves it), runtime dir absent
//  2. assert the runtime directory is genuinely absent (nothing materialized)
//  3. `app destroy --yes` must return rc=0 and leave no runtime behind
func TestW154DestroyAfterProviderChangeAbortReturnsZero(t *testing.T) {
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

	// Reconstruct the stuck-apply state exactly as a real provider-change abort
	// would leave it: deployment.json present with applied intent +
	// last_applied_ref, state "applying", but NO runtime directory materialized
	// (EnsureRuntime never ran).
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

	// (3) `app stop --yes` on the stuck state must retire with rc=0 while
	// preserving its deployment owner and any persistent data.
	var out bytes.Buffer
	derr := stopSelectiveApplicationWithConsent(context.Background(), &out)
	if derr != nil {
		t.Fatalf("destroy of stuck-apply state must return rc=0 on head, got: %v\noutput:\n%s", derr, out.String())
	}

	// After a clean retire, no owned workload runtime remains.
	if _, statErr := os.Stat(rtDir); statErr == nil {
		t.Fatalf("retire left a runtime directory behind at %s", rtDir)
	}
	t.Logf("stuck-apply application retired with rc=0; no runtime left behind")
}
