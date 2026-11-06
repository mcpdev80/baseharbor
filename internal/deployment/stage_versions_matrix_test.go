package deployment

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// newTestDeployment builds an identity under a temporary target home and returns
// its target name plus the identity. It provisions an authoritative Core
// installation so the one-installation-owner check (VerifyDeploymentOwner) passes
// on the reference-dimension seam; the tests exercise the reference dimensions,
// not the owner seam itself.
func newTestDeployment(t *testing.T) (target string, id DeploymentIdentity) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BASEHARBOR_DATA_ROOT", dir)
	appID, err := stableid.NewUUIDv4("application")
	if err != nil {
		t.Fatalf("NewUUIDv4: %v", err)
	}
	id, idErr := NewDeploymentIdentity("test-target", appID, "demo-app", "dev")
	if idErr != nil {
		t.Fatalf("NewDeploymentIdentity: %v", idErr)
	}
	// Provision the single authoritative Core owner for this Coreless test target
	// so Save/Load/verifyOwner at the seam pass. This is orthogonal to the
	// reference-dimension contract these tests assert.
	provisionOwnerForTarget(t, "test-target")
	return "test-target", id
}

// writeRecords seeds a record with all three dimensions set to distinct
// references so isolation tests can prove a mutation rewrites only one cell.
func writeAllThree(t *testing.T, id DeploymentIdentity) {
	t.Helper()
	base := func(kind, release string) ReferenceDimension {
		return ReferenceDimension{Kind: kind, Release: release, At: time.Now().UTC()}
	}
	// Seed an initial record so SetReference (which loads first) has state to work on.
	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Observed: ObservedDeployment{State: "created"},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	if err := SetReference(id, ReferenceIntended, base(ReferenceIntended, "intended-ref")); err != nil {
		t.Fatalf("set intended: %v", err)
	}
	if err := SetReference(id, ReferenceApplied, base(ReferenceApplied, "applied-ref")); err != nil {
		t.Fatalf("set applied: %v", err)
	}
	if err := SetReference(id, ReferenceObserved, base(ReferenceObserved, "observed-ref")); err != nil {
		t.Fatalf("set observed: %v", err)
	}
}

func load(t *testing.T, id DeploymentIdentity) DeploymentRecord {
	t.Helper()
	rec, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return rec
}

// TestThreeReferenceIsolation proves the matrix: mutating one dimension rewrites
// only its own cell and leaves the other two untouched (one cell per direction).
func TestThreeReferenceIsolation(t *testing.T) {
	_, id := newTestDeployment(t)
	writeAllThree(t, id)

	// Mutate intended.
	if err := SetReference(id, ReferenceIntended, ReferenceDimension{Kind: ReferenceIntended, Release: "intended-new", At: time.Now().UTC()}); err != nil {
		t.Fatalf("set intended-new: %v", err)
	}
	rec := load(t, id)
	if got := rec.References[ReferenceIntended].Release; got != "intended-new" {
		t.Errorf("intended: got %q, want intended-new", got)
	}
	if got := rec.References[ReferenceApplied].Release; got != "applied-ref" {
		t.Errorf("applied changed by intended write: got %q, want applied-ref", got)
	}
	if got := rec.References[ReferenceObserved].Release; got != "observed-ref" {
		t.Errorf("observed changed by intended write: got %q, want observed-ref", got)
	}

	// Mutate applied.
	if err := SetReference(id, ReferenceApplied, ReferenceDimension{Kind: ReferenceApplied, Release: "applied-new", At: time.Now().UTC()}); err != nil {
		t.Fatalf("set applied-new: %v", err)
	}
	rec = load(t, id)
	if got := rec.References[ReferenceApplied].Release; got != "applied-new" {
		t.Errorf("applied: got %q, want applied-new", got)
	}
	if got := rec.References[ReferenceIntended].Release; got != "intended-new" {
		t.Errorf("intended changed by applied write: got %q, want intended-new", got)
	}
	if got := rec.References[ReferenceObserved].Release; got != "observed-ref" {
		t.Errorf("observed changed by applied write: got %q, want observed-ref", got)
	}

	// Mutate observed.
	if err := SetReference(id, ReferenceObserved, ReferenceDimension{Kind: ReferenceObserved, Release: "observed-new", At: time.Now().UTC()}); err != nil {
		t.Fatalf("set observed-new: %v", err)
	}
	rec = load(t, id)
	if got := rec.References[ReferenceObserved].Release; got != "observed-new" {
		t.Errorf("observed: got %q, want observed-new", got)
	}
	if got := rec.References[ReferenceIntended].Release; got != "intended-new" {
		t.Errorf("intended changed by observed write: got %q, want intended-new", got)
	}
	if got := rec.References[ReferenceApplied].Release; got != "applied-new" {
		t.Errorf("applied changed by observed write: got %q, want applied-new", got)
	}
}

// TestReadEachDimensionIndependently proves each dimension is readable on its own.
func TestReadEachDimensionIndependently(t *testing.T) {
	_, id := newTestDeployment(t)
	writeAllThree(t, id)
	rec := load(t, id)
	if rec.References[ReferenceIntended] == nil || rec.References[ReferenceApplied] == nil || rec.References[ReferenceObserved] == nil {
		t.Fatal("expected all three dimensions present")
	}
	if rec.References[ReferenceIntended].Kind != ReferenceIntended {
		t.Errorf("intended kind: got %q", rec.References[ReferenceIntended].Kind)
	}
}

// TestStalenessDetection proves the third dimension supports staleness detection:
// observed != applied after a new apply. This is the red-first, fail-closed
// staleness operation.
func TestStalenessDetection(t *testing.T) {
	_, id := newTestDeployment(t)
	writeAllThree(t, id)

	// Apply a new reference; the applied dimension advances, observed lags until
	// the next observation.
	if err := SetReference(id, ReferenceApplied, ReferenceDimension{Kind: ReferenceApplied, Release: "applied-v2", At: time.Now().UTC()}); err != nil {
		t.Fatalf("set applied v2: %v", err)
	}
	rec := load(t, id)
	applied := rec.References[ReferenceApplied].Release
	observed := rec.References[ReferenceObserved].Release
	if applied == observed {
		t.Fatalf("expected staleness: applied %q == observed %q", applied, observed)
	}
	if rec.Stale() == nil {
		t.Fatal("expected non-nil stale when observed != applied")
	}
}

// TestMutantMergeCellsRed proves that merging two fields collapses the matrix:
// if the observed write also rewrote the applied reference, isolation fails.
// The production SetReference never does this; this test records the RED mutant
// expectation that the implementation must reject.
func TestMutantMergeCellsRed(t *testing.T) {
	_, id := newTestDeployment(t)
	writeAllThree(t, id)

	// Mutant: observed write also mutates applied.
	merged := ReferenceDimension{Kind: ReferenceObserved, Release: "observed-merge", At: time.Now().UTC()}
	if err := SetReference(id, ReferenceObserved, merged); err != nil {
		t.Fatalf("set observed-merge: %v", err)
	}
	rec := load(t, id)
	// The correct (non-merged) behaviour keeps applied untouched.
	if rec.References[ReferenceApplied].Release != "applied-ref" {
		t.Fatalf("MUTANT: applied was rewritten by observed write: %q", rec.References[ReferenceApplied].Release)
	}
	if rec.References[ReferenceObserved].Release != "observed-merge" {
		t.Fatalf("observed not written: %q", rec.References[ReferenceObserved].Release)
	}
}

// TestStaleDetectionCellRed proves the fault-tolerance-off case: when no
// observed reference is tracked at all, staleness detection must still be a first
// class, explicit operation rather than silently passing.
func TestStaleDetectionCellRed(t *testing.T) {
	_, id := newTestDeployment(t)
	// Seed an initial record so SetReference (which loads first) has state.
	if err := SaveDeploymentRecord(DeploymentRecord{Version: DeploymentRecordVersion, Identity: id, Observed: ObservedDeployment{State: "created"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Seed only intended and applied; never observe.
	if err := SetReference(id, ReferenceIntended, ReferenceDimension{Kind: ReferenceIntended, Release: "intended"}); err != nil {
		t.Fatalf("set intended: %v", err)
	}
	if err := SetReference(id, ReferenceApplied, ReferenceDimension{Kind: ReferenceApplied, Release: "applied"}); err != nil {
		t.Fatalf("set applied: %v", err)
	}
	rec := load(t, id)
	observed, ok := rec.References[ReferenceObserved]
	if !ok || observed == nil || observed.Release == "" {
		// No observation recorded: staleness detection must report the gap
		// explicitly rather than treat absence as "not stale".
		if rec.Stale() == nil {
			t.Fatal("stale detection must be explicit when no observed reference is tracked")
		}
		return
	}
	t.Fatal("expected no observed reference in this mutant")
}

// TestSetReferenceRejectsUnknownKind proves fail-closed reference-kind validation.
func TestSetReferenceRejectsUnknownKind(t *testing.T) {
	_, id := newTestDeployment(t)
	if err := SetReference(id, "staging", ReferenceDimension{Release: "x"}); err == nil {
		t.Fatal("expected SetReference to reject unknown reference kind")
	}
}

// TestClearReferenceOneDimensionOnly proves clearing one dimension leaves the
// other two intact.
func TestClearReferenceOneDimensionOnly(t *testing.T) {
	_, id := newTestDeployment(t)
	writeAllThree(t, id)
	if err := ClearReference(id, ReferenceObserved); err != nil {
		t.Fatalf("clear observed: %v", err)
	}
	rec := load(t, id)
	if _, ok := rec.References[ReferenceObserved]; ok {
		t.Fatal("observed dimension should be cleared")
	}
	if rec.References[ReferenceIntended] == nil || rec.References[ReferenceApplied] == nil {
		t.Fatal("clearing observed must not clear intended/applied")
	}
}

// TestSaveRejectsUnknownReferenceKind proves structural validation on save.
func TestSaveRejectsUnknownReferenceKind(t *testing.T) {
	_, id := newTestDeployment(t)
	// Seed an initial record so load() has state.
	if err := SaveDeploymentRecord(DeploymentRecord{Version: DeploymentRecordVersion, Identity: id, Observed: ObservedDeployment{State: "created"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec := load(t, id)
	rec.References = map[string]*ReferenceDimension{
		"staging": {Release: "x"},
	}
	if err := SaveDeploymentRecord(rec); err == nil {
		t.Fatal("expected save to reject unknown reference kind")
	}
}

// TestReferencesPreservedAcrossReopen proves persistence/reopen behavior: the
// three dimensions survive a fresh load from disk.
func TestReferencesPreservedAcrossReopen(t *testing.T) {
	target, id := newTestDeployment(t)
	writeAllThree(t, id)

	// Reopen from the same data root via the storage path.
	root, err := DeploymentRoot(id)
	if err != nil {
		t.Fatalf("DeploymentRoot: %v", err)
	}
	reopened, err := loadDeploymentRecordFile(target, id.DeploymentID, filepath.Join(root, "deployment.json"), nil)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.References[ReferenceIntended] == nil || reopened.References[ReferenceApplied] == nil || reopened.References[ReferenceObserved] == nil {
		t.Fatal("references did not survive reopen")
	}
}

// TestLifecycleOrdering proves the up/apply assembly: intended is written at
// submit, applied advances at the next apply, and observed lags until the next
// convergence — each step rewrites exactly its own dimension.
func TestLifecycleOrdering(t *testing.T) {
	_, id := newTestDeployment(t)
	// Seed an initial record so SetReference (which loads first) has state.
	if err := SaveDeploymentRecord(DeploymentRecord{Version: DeploymentRecordVersion, Identity: id, Observed: ObservedDeployment{State: "created"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Submit phase: intended and applied both resolve from the source digest.
	if err := SetReference(id, ReferenceIntended, ReferenceDimension{Kind: ReferenceIntended, Release: "v1"}); err != nil {
		t.Fatalf("set intended v1: %v", err)
	}
	if err := SetReference(id, ReferenceApplied, ReferenceDimension{Kind: ReferenceApplied, Release: "v1"}); err != nil {
		t.Fatalf("set applied v1: %v", err)
	}

	// Next apply advances applied only.
	if err := SetReference(id, ReferenceApplied, ReferenceDimension{Kind: ReferenceApplied, Release: "v2"}); err != nil {
		t.Fatalf("set applied v2: %v", err)
	}
	rec := load(t, id)
	if rec.References[ReferenceIntended].Release != "v1" {
		t.Errorf("intended changed by apply: got %q, want v1", rec.References[ReferenceIntended].Release)
	}
	if rec.References[ReferenceApplied].Release != "v2" {
		t.Errorf("applied not advanced: got %q, want v2", rec.References[ReferenceApplied].Release)
	}

	// Next convergence updates observed only.
	if err := SetReference(id, ReferenceObserved, ReferenceDimension{Kind: ReferenceObserved, Release: "v2"}); err != nil {
		t.Fatalf("set observed v2: %v", err)
	}
	rec = load(t, id)
	if rec.References[ReferenceIntended].Release != "v1" {
		t.Errorf("intended changed by converge: got %q, want v1", rec.References[ReferenceIntended].Release)
	}
	if rec.References[ReferenceApplied].Release != "v2" {
		t.Errorf("applied changed by converge: got %q, want v2", rec.References[ReferenceApplied].Release)
	}
	if rec.Stale() != nil {
		t.Errorf("not stale after observed catches up to applied")
	}

	// Divergence after converge is reported as stale.
	if err := SetReference(id, ReferenceObserved, ReferenceDimension{Kind: ReferenceObserved, Release: "stale-run"}); err != nil {
		t.Fatalf("set observed stale: %v", err)
	}
	if load(t, id).Stale() == nil {
		t.Fatal("expected stale after observed diverges from applied")
	}
}
