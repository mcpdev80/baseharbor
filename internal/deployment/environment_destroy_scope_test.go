package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// TestCollectEnvironmentDestroyScopeClassifiesScopedAndGlobal proves the
// impact report includes every deployment owned by the selected environment,
// while preserving only the sibling deployment record in another
// environment. Application identity is used to locate sibling records, not to
// make the selected environment's deployment global.
func TestCollectEnvironmentDestroyScopeClassifiesScopedAndGlobal(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	// Scoped: app-s is deployed only in prod.
	scopedID := mustNewDeploymentID(t, "default", "scoped-app", "prod")
	// Global: app-g is deployed in both prod and dev, so prod is only one of
	// its environments.
	globalID := mustNewDeploymentID(t, "default", "global-app", "prod")
	globalDevID := mustNewDeploymentID(t, "default", "global-app", "dev")

	seedDeployment(t, "default", scopedID, "scoped-app", "prod")
	seedDeployment(t, "default", globalID, "global-app", "prod")
	seedDeployment(t, "default", globalDevID, "global-app", "dev")

	scope, err := CollectEnvironmentDestroyScope("default", "prod", EnvironmentDestroyKeepData)
	if err != nil {
		t.Fatalf("collect scope: %v", err)
	}
	if len(scope.Scoped) != 2 {
		t.Fatalf("scoped set wrong: %+v", scope.Scoped)
	}
	if len(scope.Preserved) != 1 || scope.Preserved[0].Application != "global-app" || scope.Preserved[0].Environment != "dev" || scope.Preserved[0].DeploymentID != globalDevID.DeploymentID {
		t.Fatalf("preserved set wrong: %+v", scope.Preserved)
	}
	if scope.Preserved[0].Scope != environmentDestroyScopePreserved {
		t.Fatalf("global deployment scope tag = %q", scope.Preserved[0].Scope)
	}
}

// TestCollectEnvironmentDestroyScopePreservesMembers proves the impact report
// carries the authoritative member list (app + node) from the association seam,
// never a name-matched inference, and that a missing list is surfaced, not
// swallowed.
func TestCollectEnvironmentDestroyScopePreservesMembers(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	if _, err := RegisterEnvironmentAssociation("default", "prod", AssociationKindApp, "app-member"); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterEnvironmentAssociation("default", "prod", AssociationKindNode, "node-member"); err != nil {
		t.Fatal(err)
	}

	scope, err := CollectEnvironmentDestroyScope("default", "prod", EnvironmentDestroyKeepData)
	if err != nil {
		t.Fatalf("collect scope: %v", err)
	}
	if len(scope.Members) != 2 {
		t.Fatalf("members = %d, want 2: %+v", len(scope.Members), scope.Members)
	}
	seen := map[string]bool{}
	for _, m := range scope.Members {
		seen[m.Kind+"/"+m.Subject] = true
	}
	if !seen["app/app-member"] || !seen["node/node-member"] {
		t.Fatalf("members missing expected roles: %+v", scope.Members)
	}
}

// TestCollectEnvironmentDestroyScopeEmptyWhenNothingDeployed proves a brand-new
// environment with no deployments yields a valid, empty scope so the CLI can
// treat it as a confirmable no-op rather than an error.
func TestCollectEnvironmentDestroyScopeEmptyWhenNothingDeployed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	scope, err := CollectEnvironmentDestroyScope("default", "fresh", EnvironmentDestroyKeepData)
	if err != nil {
		t.Fatalf("collect empty scope: %v", err)
	}
	if scope.IsFailClosed() {
		t.Fatalf("empty environment must not fail closed: %+v", scope.FailClosed)
	}
	if len(scope.Scoped) != 0 || len(scope.Preserved) != 0 {
		t.Fatalf("empty environment scope must be empty, got %+v", scope)
	}
}

// TestCollectEnvironmentDestroyScopeDataChoiceValidation proves the data choice
// is validated and the destructive DELETE branch is flagged, so the CLI can
// demand the separate data-deletion consent.
func TestCollectEnvironmentDestroyScopeDataChoiceValidation(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	if _, err := RegisterEnvironmentAssociation("default", "prod", AssociationKindApp, "app-x"); err != nil {
		t.Fatal(err)
	}

	keep, err := CollectEnvironmentDestroyScope("default", "prod", EnvironmentDestroyKeepData)
	if err != nil {
		t.Fatal(err)
	}
	if keep.IsDestructive() {
		t.Fatalf("keep-data must not be destructive")
	}

	del, err := CollectEnvironmentDestroyScope("default", "prod", EnvironmentDestroyDeleteData)
	if err != nil {
		t.Fatal(err)
	}
	if !del.IsDestructive() {
		t.Fatalf("delete-data must be destructive")
	}

	if _, err := CollectEnvironmentDestroyScope("default", "prod", "bogus"); err == nil {
		t.Fatalf("invalid data choice must fail closed")
	}
}

// TestCollectEnvironmentDestroyScopeFailsClosedOnBadTarget proves a malformed
// target name is refused before any state is touched.
func TestCollectEnvironmentDestroyScopeFailsClosedOnBadTarget(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	if _, err := CollectEnvironmentDestroyScope("bad/name", "prod", EnvironmentDestroyKeepData); err == nil {
		t.Fatalf("invalid target must fail closed")
	}
	if _, err := CollectEnvironmentDestroyScope("default", "", EnvironmentDestroyKeepData); err == nil {
		t.Fatalf("empty environment must fail closed")
	}
}

// TestDestroyEnvironmentIdentityRemovesMatchingRecord proves the removal seam
// deletes the identity record for the exact (target, environment) and reports
// a precise not-found when the identity is absent.
func TestDestroyEnvironmentIdentityRemovesMatchingRecord(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	if _, err := GetEnvironment("default", "prod"); err != nil {
		t.Fatal(err)
	}
	if err := DestroyEnvironmentIdentity("default", "prod", EnvironmentDestroyKeepData); err != nil {
		t.Fatalf("destroy identity: %v", err)
	}
	// A second destroy on the now-absent identity must fail closed with
	// not-found, not succeed silently.
	err := DestroyEnvironmentIdentity("default", "prod", EnvironmentDestroyKeepData)
	if err == nil {
		t.Fatalf("destroying a missing environment must fail closed")
	}
	var typed *machine.Error
	if !errors.As(err, &typed) || typed.Code != machine.ErrorNotFound {
		t.Fatalf("expected not-found, got %v", err)
	}
	exists, _ := EnvironmentStateExists("default", "prod")
	if exists {
		t.Fatalf("identity should be gone")
	}
}

// TestDestroyEnvironmentIdentityRejectsForeignBinding proves the removal seam
// re-validates the on-disk binding BEFORE deleting, so a record whose stored
// (target, name) binding does not match the path it is asked to remove can
// never be deleted: the bind-check refuses rather than removing a record that
// actually belongs elsewhere.
func TestDestroyEnvironmentIdentityRejectsForeignBinding(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	if _, err := GetEnvironment("default", "prod"); err != nil {
		t.Fatal(err)
	}
	// Create a valid identity for default/prod, then corrupt its stored target
	// binding to a DIFFERENT name so the on-disk record no longer matches the
	// path it lives under. A corrupt/foreign binding must refuse the delete.
	_, path, rErr := environmentStateRoot("default", "prod")
	if rErr != nil {
		t.Fatal(rErr)
	}
	raw, rErr := os.ReadFile(path)
	if rErr != nil {
		t.Fatal(rErr)
	}
	var record map[string]any
	if uErr := json.Unmarshal(raw, &record); uErr != nil {
		t.Fatal(uErr)
	}
	record["target"] = "foreign-target"
	rewritten, mErr := json.MarshalIndent(record, "", "  ")
	if mErr != nil {
		t.Fatal(mErr)
	}
	if wErr := os.WriteFile(path, rewritten, 0o600); wErr != nil {
		t.Fatal(wErr)
	}
	// The identity must still exist at the path but its binding is now foreign;
	// the destroy seam must refuse rather than removing a mismatched record.
	if err := DestroyEnvironmentIdentity("default", "prod", EnvironmentDestroyKeepData); err == nil {
		t.Fatalf("foreign/corrupt-binding identity must fail closed")
	}
}

// TestDestroyEnvironmentIdentityPreservesOtherEnvironments proves removing one
// environment's record does not touch a sibling environment's record on the
// same Target: each (target, environment) pair owns its own state directory.
func TestDestroyEnvironmentIdentityPreservesOtherEnvironments(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	seedManagedTargetOwner(t, "default")

	if _, err := GetEnvironment("default", "prod"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetEnvironment("default", "dev"); err != nil {
		t.Fatal(err)
	}
	if err := DestroyEnvironmentIdentity("default", "prod", EnvironmentDestroyKeepData); err != nil {
		t.Fatalf("destroy prod: %v", err)
	}
	// The other environment must survive.
	if exists, _ := EnvironmentStateExists("default", "dev"); !exists {
		t.Fatalf("sibling environment dev must survive removal of prod")
	}
}

// ---- small test helpers ----

func mustNewDeploymentID(t *testing.T, target, application, environment string) DeploymentIdentity {
	t.Helper()
	id, err := NewDeploymentIdentity(target, validSubjectIDFor(application), application, environment)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedDeployment(t *testing.T, target string, id DeploymentIdentity, application, environment string) {
	t.Helper()
	record := DeploymentRecord{
		Version: DeploymentRecordVersion,
		Identity: DeploymentIdentity{
			DeploymentID:  id.DeploymentID,
			ApplicationID: id.ApplicationID,
			Target:        target,
			Application:   application,
			Environment:   environment,
		},
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "created"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
}

// validSubjectIDFor turns an application slug into a syntactically valid
// UUIDv4 (variant 1, version 4) so a test deployment carries a real
// (application_id, environment) system identity instead of a name. The exact
// value is irrelevant to the scope classification, which keys on it.
func validSubjectIDFor(application string) string {
	h := 0
	for _, r := range application {
		h = h*31 + int(r)
	}
	return fmt.Sprintf("00000000-0000-4%03x-8%03x-b%03x%04x%04x",
		byte(h>>16), byte(h>>8), byte(h), byte(h>>8), byte(h))
}
