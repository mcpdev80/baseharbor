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

// TestW154RetireWhenRuntimeDirPresentButRuntimeEnvMissing is the permanent
// regression for t-20261104-r27 (round 2, lane w156, Luna review-1382a).
//
// It proves the scope-split fix from round 2: the unconditional
// "runtime.env absent => empty bindings" absorption that round 1 had folded
// into the shared readWorkloadRuntimeEnv reader was reverted (telemetry
// _authorization.go now matches develop blob-for-blob). That shared reader is
// strict again and must stay that way — it must not fail open for other
// workload callers.
//
// The missing runtime.env in the stuck-apply state is therefore handled INSIDE
// the destroy/retire path only: stopRepositoryWorkload derives the stop set
// from ownership evidence. A runtime directory that exists but carries no
// exclusively owned runtime.env (a provider-change abort that left the dir
// without materializing runtime.env) is NOT an owned materialization, so the
// retire returns rc=0 without touching the shared reader.
//
// The test drives the REAL CLI destroy (`executeApplicationDestroyLifecycle`)
// against a stuck-apply state whose runtime directory exists but whose
// runtime.env is absent, and asserts rc=0 with no owned workload left behind.
func TestW154RetireWhenRuntimeDirPresentButRuntimeEnvMissing(t *testing.T) {
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
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: resolved.repositoryRoot(), Manifest: resolved.ManifestPath, Digest: "abort-revision-envmissing"},
		Applied: deployment.AppliedDeployment{
			Intent:          json.RawMessage(manifestBytes),
			RuntimeProvider: resolved.Target.RuntimeProvider,
			LastAppliedRef:  "abort-revision-envmissing",
			GeneratedState:  map[string]string{"deployment": resolved.DeploymentStateRoot, "state": resolved.DeploymentStateRoot + "/state"},
		},
		Observed: deployment.ObservedDeployment{State: "applying", Ready: false},
	}
	if err := deployment.SaveDeploymentRecord(pending); err != nil {
		t.Fatalf("save stuck record: %v", err)
	}

	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	// (1) The runtime directory exists, but runtime.env was never materialized.
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	if _, statErr := os.Stat(files.Env); !os.IsNotExist(statErr) {
		t.Fatalf("precondition failed: runtime.env must be missing, statErr=%v", statErr)
	}

	// (2) Ownership proof: the runtime dir exists but carries no exclusively
	// owned runtime.env. The retire must derive the stop set from this negative
	// ownership evidence, not fail closed on the missing fixed filename.
	owned, err := application.VerifyRuntimeFileOwner(files.Env)
	if err != nil {
		t.Fatalf("verify runtime owner: %v", err)
	}
	if owned {
		t.Fatalf("precondition failed: runtime.env must not be an owned file")
	}

	// (3) `app stop --yes` on the runtime-dir-present/env-missing state must
	// retire with rc=0 and leave no owned workload runtime behind.
	var out bytes.Buffer
	if derr := stopSelectiveApplicationWithConsent(context.Background(), &out); derr != nil {
		t.Fatalf("retire of runtime-dir-present/env-missing state must return rc=0, got: %v\noutput:\n%s", derr, out.String())
	}
	t.Logf("retired rc=0 with runtime dir present but no owned runtime.env; shared reader stayed strict")
}
