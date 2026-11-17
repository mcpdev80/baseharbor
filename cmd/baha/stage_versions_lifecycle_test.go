package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// resolveLifecycleFixture wires the real application up/apply lifecycle: it
// configures a Core-owned test target, writes an authored manifest, resolves the
// application, and records the pending (submit) deployment. The returned
// resolvedApplication carries its DeploymentRecord so the production recording
// functions (recordAppliedDeploymentObservation, recordObservedDeployment) write
// through the real seam instead of SetReference.
func resolveLifecycleFixture(t *testing.T) resolvedApplication {
	t.Helper()
	configureTestTarget(t)
	root := httpsAdoptionFixture(t, false)
	t.Chdir(root)
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "demo",
		Environment:   "dev",
		Workload:      application.WorkloadConfig{Components: []string{"demo-app"}},
	}
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, m)
	r, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "apply")
	if err != nil {
		t.Fatalf("resolveApplication: %v", err)
	}
	record, err := recordPendingDeployment(context.Background(), r)
	if err != nil {
		t.Fatalf("recordPendingDeployment: %v", err)
	}
	r.DeploymentRecord = &record
	return r
}

// TestLifecycleThreeDimensionsPreserved drives the real up/apply lifecycle:
// submit seeds intended+applied, applying advances applied only (intended must
// survive), and convergence writes observed only (applied AND intended must
// survive). This is the production-path matrix; the three dimensions must each
// remain readable after every transition.
func TestLifecycleThreeDimensionsPreserved(t *testing.T) {
	r := resolveLifecycleFixture(t)
	id := r.DeploymentIdentity

	// Seed phase (recordPendingDeployment) must have both intended and applied.
	seed, err := deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("seed load: %v", err)
	}
	if seed.References[deployment.ReferenceIntended] == nil {
		t.Fatal("submit must record the intended dimension")
	}
	if seed.References[deployment.ReferenceApplied] == nil {
		t.Fatal("submit must record the applied dimension")
	}

	// Apply advances applied only; intended must be preserved.
	if err := recordAppliedDeploymentObservation(context.Background(), r, application.RuntimeFiles{}, "ready", true); err != nil {
		t.Fatalf("recordAppliedDeploymentObservation: %v", err)
	}
	rec, err := deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("apply load: %v", err)
	}
	if rec.References[deployment.ReferenceApplied] == nil {
		t.Fatal("applied dropped after apply")
	}
	if rec.References[deployment.ReferenceIntended] == nil {
		t.Fatal("intended dropped after apply (the round-1 defect)")
	}
	// Observed may be absent until the first observation; that is fine here.

	// Convergence writes observed only; applied AND intended must be preserved.
	if err := recordObservedDeployment(r, "ready", true); err != nil {
		t.Fatalf("recordObservedDeployment: %v", err)
	}
	rec, err = deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("converge load: %v", err)
	}
	if rec.References[deployment.ReferenceObserved] == nil {
		t.Fatal("observed not recorded after convergence")
	}
	if rec.References[deployment.ReferenceApplied] == nil {
		t.Fatal("applied dropped on convergence (the round-1 defect)")
	}
	if rec.References[deployment.ReferenceIntended] == nil {
		t.Fatal("intended dropped on convergence")
	}
}

// TestLifecycleStaleAfterConvergence proves the round-1 fix closes the silent
// staleness pass: after convergence observed catches up to applied (not stale),
// and the applied dimension still exists so Stale() can actually compare.
func TestLifecycleStaleAfterConvergence(t *testing.T) {
	r := resolveLifecycleFixture(t)
	id := r.DeploymentIdentity
	if err := recordAppliedDeploymentObservation(context.Background(), r, application.RuntimeFiles{}, "ready", true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Observed equals applied at convergence => not stale.
	if err := recordObservedDeployment(r, "ready", true); err != nil {
		t.Fatalf("observe ready: %v", err)
	}
	if rec, err := deployment.LoadDeploymentRecord(id); err != nil {
		t.Fatalf("load: %v", err)
	} else if rec.Stale() != nil {
		t.Fatal("not stale after observed catches up to applied")
	}
}

// TestMutantMergeAppliedObservedRed is the isolation mutant: if the observed
// write ALSO rewrote the applied reference, the applied dimension would change.
// The production code never does this; this test asserts the non-merged cell.
// The mutant run (merge the two fields in recordObservedDeployment) makes this
// assertion FAIL — the exact red line is recorded in round-2.md.
func TestMutantMergeAppliedObservedRed(t *testing.T) {
	r := resolveLifecycleFixture(t)
	id := r.DeploymentIdentity
	if err := recordAppliedDeploymentObservation(context.Background(), r, application.RuntimeFiles{}, "ready", true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Snapshot the applied reference before convergence.
	before, err := deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}
	appliedBefore := before.References[deployment.ReferenceApplied].Release
	if err := recordObservedDeployment(r, "ready", true); err != nil {
		t.Fatalf("observe: %v", err)
	}
	after, err := deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	// Correct behaviour: applied is untouched by an observed write.
	if after.References[deployment.ReferenceApplied].Release != appliedBefore {
		t.Fatalf("MUTANT-RED: applied reference rewritten by observed write: %q != %q",
			after.References[deployment.ReferenceApplied].Release, appliedBefore)
	}
}

// TestStaleDetectionObserverOffRed proves the fault-tolerance-off mutant: when
// observed tracking is disabled (no observed reference recorded), Stale() must
// report the gap explicitly rather than silently pass.
func TestStaleDetectionObserverOffRed(t *testing.T) {
	r := resolveLifecycleFixture(t)
	id := r.DeploymentIdentity
	if err := recordAppliedDeploymentObservation(context.Background(), r, application.RuntimeFiles{}, "ready", true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Never observe: the observed dimension is absent.
	rec, err := deployment.LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := rec.References[deployment.ReferenceObserved]; ok {
		t.Fatal("expected no observed reference in this observer-off mutant")
	}
	// Stale detection must be explicit, not a silent pass, when no observed
	// reference is tracked.
	if rec.Stale() == nil {
		t.Fatal("STALE-RED: stale detection must report the gap when no observed reference is tracked")
	}
}
