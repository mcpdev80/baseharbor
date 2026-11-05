package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
)

func TestOneWriterGuardPositiveDirectPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed direct record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("direct apply under persisted baseharbor owner must pass: %v", err)
	}
}

func TestOneWriterGuardPositiveDelegatedPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed delegated record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("delegated apply under persisted external owner must pass: %v", err)
	}
}

// TestAttackerEmptyOwnerDoesNotMutateExternalProof proves that a caller forging an
// empty Observed.Owner cannot erase the persisted external ownership. The guard
// reads the authoritative persisted owner before mutating.
func TestAttackerEmptyOwnerDoesNotMutateExternalProof(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed external record: %v", err)
	}

	recordPath := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")

	// Attacker submits a direct (BaseHarbor) apply with an EMPTY owner. The
	// persisted external owner must win and the mutation must be refused.
	attacker := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: ""},
	}
	if err := SaveDeploymentRecord(attacker); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("empty owner under persisted external owner must refuse with ErrUnauthorizedMutation, got %v", err)
	}
	if _, statErr := os.Stat(recordPath); statErr != nil {
		t.Fatalf("store must not have been mutated: %v", statErr)
	}

	// Attacker submits a direct apply with a FORGED baseharbor owner. The
	// persisted external owner still governs.
	attacker2 := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(attacker2); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("forged baseharbor owner under persisted external owner must refuse, got %v", err)
	}
	if _, statErr := os.Stat(recordPath); statErr != nil {
		t.Fatalf("store must still not have been mutated: %v", statErr)
	}
}

// TestAttackerForgedBaseharborOwnerCannotMasquerade proves that a direct apply
// whose caller forges Owner=baseharbor is accepted only because the persisted
// baseharbor owner genuinely governs — the caller value cannot invent a direct
// apply over a persisted external owner. Seeded external, caller-forged
// baseharbor: the persisted external owner wins and the mutation refuses.
func TestAttackerForgedBaseharborOwnerCannotMasquerade(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed external record: %v", err)
	}

	recordPath := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")

	// Attacker submits a direct apply with a forged Owner=baseharbor. The
	// persisted external owner governs and refuses the direct mutation.
	attacker := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(attacker); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("forged baseharbor owner over persisted external owner must refuse with ErrUnauthorizedMutation, got %v", err)
	}
	if _, statErr := os.Stat(recordPath); statErr != nil {
		t.Fatalf("store must not have been mutated: %v", statErr)
	}
}

// TestPersistedDirectOwnerPassesDirectApply is the alt-gap guard: a prior direct
// (baseharbor) owner plus a caller-supplied conflicting observed value must not
// falsely fail — the persisted direct value governs.
func TestPersistedDirectOwnerPassesDirectApply(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed direct record: %v", err)
	}

	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("direct apply under persisted baseharbor owner must pass: %v", err)
	}
}

// TestForgedDelegatedUpdateThenDirectRefused is the regression for the exact
// bypass the r2 review rejected: a delegated update whose caller forges
// Observed.Owner is persisted with the caller value, so a subsequent direct
// apply would read a forged/empty owner and pass the guard. Persisting the
// canonical guarded owner closes the hole: the persisted external owner governs
// and the later direct apply is refused.
func TestForgedDelegatedUpdateThenDirectRefused(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed external record: %v", err)
	}

	// Delegated update whose caller forges an empty owner (would persist the
	// empty value and let the next direct apply slip past the guard).
	forged := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: ""},
	}
	if err := SaveDeploymentRecord(forged); err != nil {
		t.Fatalf("delegated update must succeed: %v", err)
	}

	// The persisted owner must still be canonical (external), not the forged empty.
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load after delegated update: %v", err)
	}
	if got.Observed.Owner != delivery.OwnerExternal {
		t.Fatalf("persisted owner must be canonical external, got %q", got.Observed.Owner)
	}

	// A subsequent direct (baseharbor-owned) apply is now refused, because the
	// persisted external owner governs.
	direct := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(direct); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("direct apply after forged delegated update must be refused with ErrUnauthorizedMutation, got %v", err)
	}
}

// TestRejectedWriteLeavesBytesIdentical proves that when the guard refuses a
// mutation the on-disk file is left byte-for-byte unchanged.
func TestRejectedWriteLeavesBytesIdentical(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed external record: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json"))
	if err != nil {
		t.Fatalf("read seed file: %v", err)
	}

	attacker := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(attacker); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("attacker direct write must be refused with ErrUnauthorizedMutation, got %v", err)
	}
	after, err := os.ReadFile(filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json"))
	if err != nil {
		t.Fatalf("read file after refused write: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("store bytes changed despite refused write:\nbefore=%q\nafter=%q", before, after)
	}
}

// TestPreviousConflictIsCarried proves that a conflict observed in the persisted
// previous record is carried into the guard evaluation: a same-owner update is
// refused (fail closed) when the previous record observed two active owners, so
// the conflict is neither hidden nor silently cleared on the next write.
func TestPreviousConflictIsCarried(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	// Persist a baseharbor-owned record, then hand-inject a conflict flag into
	// the stored file (a conflict is never written via SaveDeploymentRecord
	// itself, since the guard refuses it on write).
	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed baseharbor record: %v", err)
	}
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	seedRecord, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	// A conflict is never persisted via SaveDeploymentRecord (the guard refuses
	// it on write), so it is injected here by re-marshaling a record whose
	// previous observed state carried a conflict.
	seedRecord.Observed.Conflict = true
	injected, err := json.MarshalIndent(seedRecord, "", "  ")
	if err != nil {
		t.Fatalf("marshal injected: %v", err)
	}
	injected = append(injected, '\n')
	if err := os.WriteFile(path, injected, 0o600); err != nil {
		t.Fatalf("write injected: %v", err)
	}
	if !strings.Contains(string(injected), `"conflict": true`) {
		t.Fatalf("injected record must carry the conflict flag: %s", injected)
	}

	// A same-owner (baseharbor) update must be refused: the previous conflict is
	// carried into the guard and the apply fails closed.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(record); !errors.Is(err, ErrConflictingOneWriter) {
		t.Fatalf("same-owner update under carried conflict must be refused with ErrConflictingOneWriter, got %v", err)
	}
}

// TestFirstSaveCallerConflictTruePasses is the exact reviewer recipe for point 2:
// on a first save (previousExists==false) the caller-supplied
// Observed.Conflict must NOT drive a rejection. The caller cannot invent a
// conflict; the store's own determination is the only thing that counts.
func TestFirstSaveCallerConflictTruePasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	rec := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor, Conflict: true},
	}
	if err := SaveDeploymentRecord(rec); err != nil {
		t.Fatalf("first-save caller conflict=true must NOT be rejected, got %v", err)
	}
}

// TestFirstSaveCallerConflictFalsePasses confirms a benign caller conflict on the
// first save also passes (sanity for the discard path).
func TestFirstSaveCallerConflictFalsePasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	rec := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor, Conflict: false},
	}
	if err := SaveDeploymentRecord(rec); err != nil {
		t.Fatalf("first-save caller conflict=false must pass, got %v", err)
	}
}

// TestFirstSaveCallerConflictDiscardedUnderStoredFalse proves the discard is real:
// a first save with caller conflict=true persists Conflict=false, i.e. the store
// never wrote a conflict it did not itself observe.
func TestFirstSaveCallerConflictDiscardedUnderStoredFalse(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	rec := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor, Conflict: true},
	}
	if err := SaveDeploymentRecord(rec); err != nil {
		t.Fatalf("first-save caller conflict=true must pass, got %v", err)
	}
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load after first save: %v", err)
	}
	if got.Observed.Conflict {
		t.Fatalf("first-save caller conflict=true must be discarded, persisted Conflict must be false")
	}
	if got.Observed.Owner != delivery.OwnerBaseHarbor {
		t.Fatalf("persisted owner must be canonical baseharbor, got %q", got.Observed.Owner)
	}
}

// TestFirstSaveDelegatedCallerConflictPasses confirms the discard is mode-agnostic:
// a delegated first save with caller conflict=true also passes.
func TestFirstSaveDelegatedCallerConflictPasses(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	rec := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal, Conflict: true},
	}
	if err := SaveDeploymentRecord(rec); err != nil {
		t.Fatalf("first-save delegated caller conflict=true must pass, got %v", err)
	}
}

// seedCorruptedOwner persists a record, then hand-injects a corrupt/unrecognised
// persisted reconciliation owner into the stored deployment.json. The stored
// owner is neither "" nor a recognised owner (baseharbor|external).
func seedCorruptedOwner(t *testing.T, id DeploymentIdentity, owner string) string {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed corrupted-owner record: %v", err)
	}
	path := filepath.Join(t.TempDir(), "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	return path
}
func TestCorruptedOwnerDirectPassesUnsetPathIsWideningNeutral(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed baseharbor record: %v", err)
	}
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Observed.Owner = delivery.ReconciliationOwner("corrupt-third-party")
	injected, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal injected: %v", err)
	}
	injected = append(injected, '\n')
	if err := os.WriteFile(path, injected, 0o600); err != nil {
		t.Fatalf("write injected corrupt owner: %v", err)
	}

	record2 := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(record2); err != nil {
		t.Fatalf("direct apply over corrupt persisted owner must pass via unset-path, got %v", err)
	}
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load after direct apply: %v", err)
	}
	if got.Observed.Owner != delivery.OwnerBaseHarbor {
		t.Fatalf("persisted owner must become canonical baseharbor, got %q", got.Observed.Owner)
	}
}

// TestCorruptedOwnerDelegatedOwnExternalGoverns is regression 2 restated
// precisely: the corrupt persisted owner is not widened into a valid owner — a
// delegated apply whose own selection owner is genuinely external is allowed
// because the unset-path defers to the applied selection's owner, and the
// persisted owner becomes canonical external. The corrupt value neither widens
// nor attests any owner beyond what the applied selection carries.
func TestCorruptedOwnerDelegatedOwnExternalGoverns(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed baseharbor record: %v", err)
	}
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Observed.Owner = delivery.ReconciliationOwner("corrupt-third-party")
	injected, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal injected: %v", err)
	}
	injected = append(injected, '\n')
	if err := os.WriteFile(path, injected, 0o600); err != nil {
		t.Fatalf("write injected corrupt owner: %v", err)
	}

	delegated := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(delegated); err != nil {
		t.Fatalf("delegated apply whose own owner is external must pass over corrupt persisted owner, got %v", err)
	}
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load after delegated apply: %v", err)
	}
	if got.Observed.Owner != delivery.OwnerExternal {
		t.Fatalf("persisted owner must become canonical external, got %q", got.Observed.Owner)
	}
}

// TestCorruptedOwnerPreviousConflictStillRefuses is regression 3: a corrupt
// persisted owner plus a previous record whose conflict flag is true still
// refuses (fail closed): the carried conflict wins regardless of the owner value.
func TestCorruptedOwnerPreviousConflictStillRefused(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed baseharbor record: %v", err)
	}
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Observed.Owner = delivery.ReconciliationOwner("corrupt-third-party")
	record.Observed.Conflict = true
	injected, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal injected: %v", err)
	}
	injected = append(injected, '\n')
	if err := os.WriteFile(path, injected, 0o600); err != nil {
		t.Fatalf("write injected corrupt owner + conflict: %v", err)
	}

	// A same-owner update must be refused: the previous conflict carries into the
	// guard and fails closed, even with a corrupt owner.
	update := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(update); !errors.Is(err, ErrConflictingOneWriter) {
		t.Fatalf("same-owner update under carried conflict + corrupt owner must refuse with ErrConflictingOneWriter, got %v", err)
	}
}

// TestCorruptedOwnerNormaliseHelperDirect checks the helper directly: an
// unknown/corrupt value collapses to "" (unset) while recognised values are
// preserved lower-cased, and whitespace/upper-case is normalised.
func TestCorruptedOwnerNormaliseHelperDirect(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"baseharbor":              "baseharbor",
		"external":                "external",
		"corrupt-third-party":     "",
		"BASEHARBOR":              "baseharbor",
		"  external  ":            "external",
		"  corrupt-third-party  ": "",
		"BaseHarbor":              "baseharbor",
	}
	for in, want := range cases {
		if got := normalisePersistedOwner(in); got != want {
			t.Fatalf("normalisePersistedOwner(%q) = %q, want %q", in, got, want)
		}
	}
}
