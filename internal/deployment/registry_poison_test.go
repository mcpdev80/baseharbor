package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCollisionRecord lives in target_test.go: it persists a minimal record
// by direct file write so a read-side poison cell can seed a state the write
// seam itself now refuses (0427-175).

// TestSaveDeploymentRecordRefusesDuplicateApplicationID is the write-seam
// regression (0427-175, bug hunt hp4 BF-01): a first registration under a
// brand-new deployment uuid that claims an (application_id, environment) pair
// an OTHER deployment already owns is refused with ErrDuplicateApplicationID
// before any state is written.
func TestSaveDeploymentRecordRefusesDuplicateApplicationID(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	provisionOwnerForTarget(t, "docker-dev")
	winner := testDeploymentIdentity(t, "docker-dev", "alpha", "dev")
	if err := SaveDeploymentRecord(DeploymentRecord{
		Identity: winner,
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}); err != nil {
		t.Fatal(err)
	}
	racer := testDeploymentIdentity(t, "docker-dev", "alpha-racing", "dev")
	err := SaveDeploymentRecord(DeploymentRecord{
		Identity: racer,
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	})
	if !errors.Is(err, ErrDuplicateApplicationID) {
		t.Fatalf("a second registration of one application_id must be refused, got %v", err)
	}
	if !strings.Contains(err.Error(), winner.DeploymentID) {
		t.Fatalf("the refusal must name the deployment that already owns the identity: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(testDeploymentRoot(t, racer))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the refused registration must not create any deployment state: %v", statErr)
	}
}

// TestSaveDeploymentRecordSameIdentityStaysIdempotent pins the other half of
// the guard: re-saving the SAME deployment uuid (the durable apply retry path)
// must keep working — the refusal keys on a FOREIGN claim, never on the
// record's own directory.
func TestSaveDeploymentRecordSameIdentityStaysIdempotent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	provisionOwnerForTarget(t, "docker-dev")
	id := testDeploymentIdentity(t, "docker-dev", "alpha", "dev")
	for i := 0; i < 3; i++ {
		if err := SaveDeploymentRecord(DeploymentRecord{
			Identity: id,
			Applied:  AppliedDeployment{RuntimeProvider: "docker"},
		}); err != nil {
			t.Fatalf("idempotent re-save %d: %v", i, err)
		}
	}
}

// TestListDeploymentsReportsRegistryConflictOnPoisonedTarget proves the
// read-seam half (0427-175): a registry that already holds the poison — because
// an older binary raced two registrations through — surfaces ONE structured,
// actionable RegistryConflictError instead of silently picking one record.
func TestListDeploymentsReportsRegistryConflictOnPoisonedTarget(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	provisionOwnerForTarget(t, "docker-dev")
	first := testDeploymentIdentity(t, "docker-dev", "alpha", "dev")
	second := testDeploymentIdentity(t, "docker-dev", "renamed-alpha", "dev")
	writeCollisionRecord(t, first)
	writeCollisionRecord(t, second)

	_, err := ListDeployments("docker-dev")
	if !errors.Is(err, ErrRegistryConflict) {
		t.Fatalf("the inventory seam must expose the conflict sentinel: %v", err)
	}
	var conflict *RegistryConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("the conflict must be structured: %T %v", err, err)
	}
	if conflict.Key.ApplicationID != testApplicationID || conflict.Key.Environment != "dev" {
		t.Fatalf("the conflict must name the poisoned identity group: %+v", conflict.Key)
	}
	joined := conflict.Error()
	if !strings.Contains(joined, first.DeploymentID) || !strings.Contains(joined, second.DeploymentID) {
		t.Fatalf("the conflict must name BOTH colliding deployments for the --uuid repair: %s", joined)
	}
	if !strings.Contains(joined, "baha app destroy --uuid") || !strings.Contains(joined, "--yes") {
		t.Fatalf("the conflict must name the verified destroy command (lesson 162): %s", joined)
	}

	// FindDeployment routes through the same seam and must refuse to pick.
	if _, _, err := FindDeployment("docker-dev", testApplicationID, "dev"); !errors.Is(err, ErrRegistryConflict) {
		t.Fatalf("FindDeployment must refuse a poisoned registry, got %v", err)
	}
}

// TestListDeploymentsForDisplayToleratesThePoison pins the repair door: the
// display listing (what 'baha app list' uses) still shows both twins with
// their uuids on a poisoned target, so the operator can SEE what to destroy.
func TestListDeploymentsForDisplayToleratesThePoison(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	provisionOwnerForTarget(t, "docker-dev")
	first := testDeploymentIdentity(t, "docker-dev", "alpha", "dev")
	second := testDeploymentIdentity(t, "docker-dev", "renamed-alpha", "dev")
	writeCollisionRecord(t, first)
	writeCollisionRecord(t, second)

	records, _, err := ListDeploymentsForDisplay("docker-dev")
	if err != nil {
		t.Fatalf("the display listing must tolerate the poison: %v", err)
	}
	seen := map[string]bool{}
	for _, record := range records {
		seen[record.Identity.DeploymentID] = true
	}
	if !seen[first.DeploymentID] || !seen[second.DeploymentID] {
		t.Fatalf("app list must show both colliding twins: %v", seen)
	}
	// The exported detector gives the command guards the production rule over
	// exactly this tolerant listing.
	if err := RefuseApplicationIDConflictOnDisplay("docker-dev", records); !errors.Is(err, ErrRegistryConflict) {
		t.Fatalf("the detector must find what the listing carried: %v", err)
	}
}

// TestDetectApplicationIDCollisionsIsOrderIndependent locks the shared
// detection rule: one group per (application_id, environment) with sorted ids;
// distinct environments and empty application ids are never collisions.
func TestDetectApplicationIDCollisionsIsOrderIndependent(t *testing.T) {
	target := "docker-dev"
	byKeyFirst := []DeploymentRecord{
		{Identity: DeploymentIdentity{Target: target, ApplicationID: "app-1", Environment: "dev", DeploymentID: "d-2"}},
		{Identity: DeploymentIdentity{Target: target, ApplicationID: "app-1", Environment: "dev", DeploymentID: "d-1"}},
		{Identity: DeploymentIdentity{Target: target, ApplicationID: "app-1", Environment: "prod", DeploymentID: "d-3"}},
		{Identity: DeploymentIdentity{Target: target, ApplicationID: "", Environment: "dev", DeploymentID: "d-4"}},
		{Identity: DeploymentIdentity{Target: target, ApplicationID: "", Environment: "dev", DeploymentID: "d-5"}},
		{Identity: DeploymentIdentity{Target: "other", ApplicationID: "app-1", Environment: "dev", DeploymentID: "d-6"}},
	}
	collisions := applicationIDCollisionsIn(byKeyFirst, target)
	if len(collisions) != 1 {
		t.Fatalf("exactly one colliding group expected, got %v", collisions)
	}
	key := DeploymentIdentity{Target: target, ApplicationID: "app-1", Environment: "dev"}
	ids, ok := collisions[key]
	if !ok || len(ids) != 2 || ids[0] != "d-1" || ids[1] != "d-2" {
		t.Fatalf("collision ids must be deterministic and sorted: %v %v", ok, ids)
	}
	// Reversed input order yields the identical report.
	reversed := make([]DeploymentRecord, len(byKeyFirst))
	for i := range byKeyFirst {
		reversed[i] = byKeyFirst[len(byKeyFirst)-1-i]
	}
	again := applicationIDCollisionsIn(reversed, target)
	if len(again) != 1 || again[key][0] != "d-1" || again[key][1] != "d-2" {
		t.Fatalf("detection must be order-independent: %v", again)
	}
}

// TestRegistryConflictParsersRoundTrip pins the message contract the command
// surface falls back on when an error crossed a serialization boundary: the
// sentinel classifier, the id extractor and the identity-key parser must all
// read the canonical RegistryConflictError text.
func TestRegistryConflictParsersRoundTrip(t *testing.T) {
	conflict := &RegistryConflictError{
		Target:        "docker-dev",
		Key:           DeploymentIdentity{Target: "docker-dev", ApplicationID: "app-1", Environment: "dev"},
		DeploymentIDs: []string{"d-1", "d-2"},
	}
	if !IsRegistryConflict(conflict) || !IsRegistryConflict(errors.New(ErrRegistryConflict.Error()+": wrapped")) {
		t.Fatal("the classifier must accept the structured and the sentinel-text forms")
	}
	if IsRegistryConflict(errors.New("unrelated")) {
		t.Fatal("an unrelated error must not classify as a registry conflict")
	}
	wrapped := errors.Join(errors.New("context"), conflict)
	ids := RegistryConflictIDs(wrapped)
	if len(ids) != 2 || ids[0] != "d-1" || ids[1] != "d-2" {
		t.Fatalf("the id extractor must recover both colliding deployments through any wrap: %v", ids)
	}
	applicationID, environment, ok := RegistryConflictKey(wrapped)
	if !ok || applicationID != "app-1" || environment != "dev" {
		t.Fatalf("the key parser must recover the poisoned identity: %q %q %v", applicationID, environment, ok)
	}
}

func testDeploymentRoot(t *testing.T, id DeploymentIdentity) string {
	t.Helper()
	root, err := DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
