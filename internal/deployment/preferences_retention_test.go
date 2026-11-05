package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// TestReviewerRetainsPreferences is the exact reviewer-repro from review-1333e:
// seed a valid nonempty preference array, save the same deployment with nil
// preferences, reload, assert identical. This was the data-loss regression.
func TestReviewerRetainsPreferences(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record with preferences: %v", err)
	}

	// Same deployment, nil preferences (a lifecycle save that does not carry
	// preferences). The retained array must survive.
	nilPrefs := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(nilPrefs); err != nil {
		t.Fatalf("save with nil preferences: %v", err)
	}

	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("reload after nil-prefs save: %v", err)
	}
	var before, after []map[string]string
	if err := json.Unmarshal(seed.DesiredProviderPreferences, &before); err != nil {
		t.Fatalf("unmarshal seed prefs: %v", err)
	}
	if err := json.Unmarshal(got.DesiredProviderPreferences, &after); err != nil {
		t.Fatalf("unmarshal reloaded prefs: %v", err)
	}
	if len(after) != len(before) || after[0]["profile"] != before[0]["profile"] {
		t.Fatalf("preferences lost on nil save, seed=%v reloaded=%v", before, after)
	}
}

// TestNilRetentionDifferentOwnerRejected proves that a nil save against a
// previous record owned by a DIFFERENT deployment owner retains nothing and is
// refused, not silently overwritten.
func TestNilRetentionDifferentOwnerRejected(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Inject a different application/environment into the stored file to simulate
	// a previous record owned by a different deployment owner.
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Identity.ApplicationID = "22222222-2222-4222-8222-222222222222"
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}); err == nil {
		t.Fatalf("nil save against different owner must be refused, got nil error")
	}
}

// TestExplicitEmptyArrayReplaced proves that an explicit empty array [] is
// treated as a replacement and overwrites previously retained preferences.
func TestExplicitEmptyArrayReplaced(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	// Explicit empty array is an explicit replacement.
	empty := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[]`),
	}
	if err := SaveDeploymentRecord(empty); err != nil {
		t.Fatalf("save explicit empty array: %v", err)
	}
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var after []map[string]string
	if err := json.Unmarshal(got.DesiredProviderPreferences, &after); err != nil {
		t.Fatalf("unmarshal reloaded prefs: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("explicit empty array must clear retained prefs, got %v", after)
	}
}

// TestInvalidPreferencesRejected proves that invalid (non-array JSON) desired
// provider preferences are rejected on save.
func TestInvalidPreferencesRejected(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	bad := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`{"not":"an array"`),
	}
	if err := SaveDeploymentRecord(bad); err == nil {
		t.Fatalf("invalid preferences must be rejected, got nil error")
	}
}

// TestForgedOwnerBypassStillHolds is the existing forged-owner regression kept
// intact: an external reconciler remains authoritative across a forged
// delegated update and a subsequent direct save is refused.
func TestForgedOwnerBypassStillHolds(t *testing.T) {
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
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Observed.Owner != delivery.OwnerExternal {
		t.Fatalf("persisted owner must stay canonical external, got %q", got.Observed.Owner)
	}
	direct := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(direct); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("direct apply after forged delegated update must be refused, got %v", err)
	}
}

// TestCallerConflictNotTrusted proves the caller-supplied Observed.Conflict is
// discarded: with previous.conflict=false and no persisted conflict, a same
// owner update is NOT rejected because of the caller conflict field alone. The
// semantics are previous-conflict OR store-own-revelation; caller-feld ignored.
func TestCallerConflictNotTrusted(t *testing.T) {
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
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	// Same-owner update with a caller-supplied conflict flag but a persisted
	// previous.conflict=false. The guard must NOT reject on the caller field
	// alone: previous-conflict OR store-own-revelation governs, caller ignored.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor, Conflict: true},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("same-owner update must not be rejected on caller-conflict alone, got %v", err)
	}
}

// TestPreviousConflictStillFailsClosed re-confirms that a persisted previous
// conflict is honored: same-owner update under a carried conflict refuses.
func TestPreviousConflictStillFailsClosed(t *testing.T) {
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
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Observed.Conflict = true
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("write injected: %v", err)
	}

	same := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(same); !errors.Is(err, ErrConflictingOneWriter) {
		t.Fatalf("same-owner update under carried conflict must refuse with ErrConflictingOneWriter, got %v", err)
	}
}

// TestRetentionPreviousInvalidSelectionFailsClosed proves that a nil save against
// a previous record that the full loader rejects (invalid delivery selection)
// fails closed: the save is refused and the previously owned preferences stay.
func TestRetentionPreviousInvalidSelectionFailsClosed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Corrupt the persisted selection so the full loader rejects the previous
	// record on re-read (invalid delivery selection).
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.Delivery = delivery.Selection{Mode: delivery.ModeDirect, Provider: "not a valid provider"}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	// Nil save against the corrupt previous record must be refused, not silently
	// succeed and drop the retained preferences.
	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}); err == nil {
		t.Fatalf("nil save against previous invalid-selection record must fail closed, got nil error")
	}
}

// byteStabilityFixture seeds a valid previous record with a known preference
// array and returns the absolute path to the persisted deployment.json so a
// test can mutate or remove the file between the guard-load and the retention
// load. It uses the shared helper identity (UUID deployment ID) and the exact
// path layout that SaveDeploymentRecord writes to.
func byteStabilityFixture(t *testing.T) (id DeploymentIdentity, path string) {
	t.Helper()
	id = testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner for this target AFTER env setup
	// so the deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)
	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed byte-stability fixture: %v", err)
	}
	path = filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	return id, path
}

// nilSave builds a nil-preferences lifecycle save (the trigger for the retention
// re-read path).
func nilSave(id DeploymentIdentity) DeploymentRecord {
	return DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
}

// TestRetentionInvalidSelectionByteStable proves that a nil save against a
// previous record whose persisted delivery selection is invalid fails closed:
// the save is refused and the previously owned preference bytes are unchanged.
func TestRetentionInvalidSelectionByteStable(t *testing.T) {
	id, path := byteStabilityFixture(t)
	// Corrupt the persisted selection so the retention full-loader rejects it.
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.Delivery = delivery.Selection{Mode: delivery.ModeDirect, Provider: "not a valid provider"}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if err := SaveDeploymentRecord(nilSave(id)); err == nil {
		t.Fatalf("nil save against previous invalid-selection record must fail closed, got nil error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("bytes changed after refused save:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestRetentionInvalidRemoteProjectByteStable proves that a nil save against a
// previous record rejected by the full loader for an invalid remote-project
// binding fails closed and leaves the stored bytes untouched.
func TestRetentionInvalidRemoteProjectByteStable(t *testing.T) {
	id, path := byteStabilityFixture(t)
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.RemoteProject = &targetsession.ProjectRecord{
		Scope: targetenrollment.Scope{
			TargetID: "other-target",
			Runtime:  "k8s",
		},
	}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if err := SaveDeploymentRecord(nilSave(id)); err == nil {
		t.Fatalf("nil save against previous invalid-remote-project record must fail closed, got nil error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("bytes changed after refused save:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestRetentionVanishedRecordFailsClosed proves that a nil save against a
// previous record that vanishes between the guard-read and the retention-read
// fails closed through the REAL SaveDeploymentRecord path. It uses the
// package-internal seam testHookAfterGuardRead, which fires after the guard has
// read the persisted previous record (previousExists=true, owner established)
// and immediately before the retention re-read: the seam deletes the on-disk
// record so the retention full-loader returns os.ErrNotExist on the in-process
// second read. The save must be refused (fail-closed) and the on-disk file must
// stay absent (byte-proof: ENOENT is only acceptable because the record was
// already gone after the guard confirmed it existed). This test fails against
// the pre-fix code that swallowed the ENOENT loader error (silently dropping
// the owned preferences).
func TestRetentionVanishedRecordFailsClosed(t *testing.T) {
	id, path := byteStabilityFixture(t)

	// Anchor the seam: it fires once, after the guard-read, before the retention
	// read. Remove the previous record there to simulate a vanish between reads.
	removed := false
	testHookAfterGuardRead = func() {
		if removed {
			return
		}
		removed = true
		if err := os.Remove(path); err != nil {
			t.Fatalf("seam remove record: %v", err)
		}
	}
	defer func() { testHookAfterGuardRead = nil }()

	before, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat previous record before save: %v", statErr)
	}
	if before == nil {
		t.Fatalf("fixture previous record must exist before save")
	}

	// Real SaveDeploymentRecord path: guard reads the previous (exists), seam
	// deletes it, the retention re-read surfaces ENOENT and the save fails closed.
	if SaveDeploymentRecord(nilSave(id)) == nil {
		t.Fatalf("nil save against vanished previous record must fail closed, got nil error")
	}

	// Byte-proof: the record file must remain absent — the vanished file was
	// deleted after the guard confirmed it existed, so nothing may be rewritten.
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("after refused save the vanished record must stay absent (ENOENT), got stat err=%v", statErr)
	}
}

// TestRetentionVanishedRecordFailsClosedViaSave exercises the vanished-record
// fail-closed path through the full SaveDeploymentRecord entry point. It seeds a
// previous record, then re-seeds the SAME path with a record whose full-loader
// read is corrupted (invalid selection) so the retention re-read fails; this
// proves the ENOENT-and-other-loader-failures branch is reached via a real save
// and leaves the stored bytes unchanged. The pure vanished-file race is covered
// by the direct two-read sequence in TestRetentionVanishedRecordFailsClosed.
func TestRetentionVanishedRecordFailsClosedViaSave(t *testing.T) {
	id, path := byteStabilityFixture(t)
	// Corrupt the remote-project binding so the retention full-loader rejects
	// the previous record (loader-failure branch, same fail-closed path as
	// ENOENT). ValidateRemoteProject catches a binding that does not match the
	// target/runtime, and Normalize does not repair it.
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.RemoteProject = &targetsession.ProjectRecord{
		Scope: targetenrollment.Scope{
			TargetID: "other-target",
			Runtime:  "k8s",
		},
	}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}
	if err := SaveDeploymentRecord(nilSave(id)); err == nil {
		t.Fatalf("nil save against previous record that the loader rejects must fail closed, got nil error")
	}
}

// TestRetentionFirstSaveWithoutPreviousAllowed proves the control: the very
// first save with no previous record and nil preferences is permitted and not
// refused by the retention branch (the negative control for the ENOENT fix).
func TestRetentionFirstSaveWithoutPreviousAllowed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	if err := SaveDeploymentRecord(nilSave(id)); err != nil {
		t.Fatalf("first save without previous must be allowed, got %v", err)
	}
}

// TestRetentionIdentityMismatchByteStable proves that a nil save against a
// previous record with a different ApplicationID (identity mismatch) fails
// closed and leaves the stored bytes unchanged.
func TestRetentionIdentityMismatchByteStable(t *testing.T) {
	id, path := byteStabilityFixture(t)
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Identity.ApplicationID = "22222222-2222-4222-8222-222222222222"
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	if err := SaveDeploymentRecord(nilSave(id)); err == nil {
		t.Fatalf("nil save against previous with different applicationID must fail closed, got nil error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("bytes changed after refused save:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestRetentionPreviousInvalidRemoteProjectFailsClosed proves that a nil save
// against a previous record rejected by the full loader for an invalid
// remote-project binding fails closed.
func TestRetentionPreviousInvalidRemoteProjectFailsClosed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)
	// Seed a genuine authoritative Core owner AFTER env setup so the
	// deployment-owner seam resolves it from the test's temp data root.
	seedManagedTargetOwner(t, id.Target)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Inject a remote-project binding that does not match target/runtime so the
	// full loader rejects the previous record.
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.RemoteProject = &targetsession.ProjectRecord{
		Scope: targetenrollment.Scope{
			TargetID: "other-target",
			Runtime:  "k8s",
		},
	}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}); err == nil {
		t.Fatalf("nil save against previous invalid-remote-project record must fail closed, got nil error")
	}
}
