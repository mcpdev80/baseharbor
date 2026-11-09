package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// seedOrphanDeploymentRecord writes a deployment.json for the given target whose
// installation owner is absent (ownerless Target) and whose source repository is
// unavailable, reproducing a target-delete orphan without a live Core. The
// deployment directory is named with the SAME UUID the record embeds, so it is a
// genuine collectable orphan (decode-uuid == folder name).
func seedOrphanDeploymentRecord(t *testing.T, target string) string {
	t.Helper()
	deploymentID := "11111111-1111-4111-8111-111111111111"
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", deploymentID)
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	intent, _ := json.Marshal(application.New("orphanapp", "dev", false, false, false))
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  deploymentID,
			ApplicationID: application.MustNewApplicationID(),
			Target:        target,
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: "/nonexistent/repo",
			Manifest:   "/nonexistent/baseharbor.yaml",
		},
		Applied: deployment.AppliedDeployment{Intent: intent},
	}
	path := filepath.Join(depDir, "deployment.json")
	if err := os.WriteFile(path, mustJSON(t, record), 0600); err != nil {
		t.Fatal(err)
	}
	return deploymentID
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestTargetDeleteRefusesWhileCollectableOrphansAttached verifies the smallest
// honest fix for 0427-165: detach refuses (does not silently remove) while a
// deployment record that would become uncollectable is attached, naming the
// record UUID and the collect path, and leaves the configuration untouched.
func TestTargetDeleteRefusesWhileCollectableOrphansAttached(t *testing.T) {
	target := configureTestTarget(t)
	// configureTestTarget registers a Core installation for the target; remove its
	// runtime owner so the seeded record is ownerless (the exact orphan shape).
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(stateRoot, "runtime")); err != nil {
		t.Fatal("remove runtime owner for ownerless orphan", err)
	}
	id := seedOrphanDeploymentRecord(t, target.Name)
	ctx := context.Background()

	plan, err := buildTargetDefinitionChangePlan(ctx, target.Name, "")
	if err == nil {
		t.Fatal("detach accepted a collectable orphan without refusal")
	}
	classified := machine.Classify(err)
	if classified == nil || classified.CauseCode != "target_definition_collectable_orphans" {
		t.Fatalf("orphan detach refusal not surfaced as collectable-orphan error: %#v", err)
	}
	if !strings.Contains(err.Error(), id) {
		t.Fatalf("orphan refusal did not name the attached record UUID %q: %v", id, err)
	}
	if !strings.Contains(err.Error(), "destroy --all") {
		t.Fatalf("orphan refusal did not point at the collect path: %v", err)
	}

	// The configuration entry must be preserved (a visible refusal, not silent removal).
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Targets[target.Name]; !ok {
		t.Fatal("orphan refusal removed the configuration entry instead of refusing")
	}
	if _, err := os.Stat(plan.RecoveryFile); !os.IsNotExist(err) {
		t.Fatal("orphan refusal created a recovery file")
	}
}

// TestTargetDeleteProceedsWithoutOrphans verifies the guard is silent for a clean
// Target: no collectable orphans means the plan builds and reaches the normal
// provider-consequence disclosure, so the operator can detach.
func TestTargetDeleteProceedsWithoutOrphans(t *testing.T) {
	target := configureTestTarget(t)
	orphans, err := deployment.CollectableOrphans(target.Name)
	if err != nil {
		t.Fatalf("clean Target reported orphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("clean Target must have no collectable orphans, got %d", len(orphans))
	}
	ctx := context.Background()
	plan, err := buildTargetDefinitionChangePlan(ctx, target.Name, "")
	if err != nil {
		t.Fatalf("clean Target refused detach: %v", err)
	}
	if plan.Scope.Attention != "Achtung" || plan.Scope.ConfirmationsRequired != 2 {
		t.Fatal("clean detach did not reach the standard two-stage consent disclosure")
	}
}

// TestTargetDeleteRefusalIsJSONFriendly verifies the refusal is structurally
// stable under -o json (machine-readable), so automation can act on the hint.
func TestTargetDeleteRefusalIsJSONFriendly(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(filepath.Join(stateRoot, "runtime"))
	seedOrphanDeploymentRecord(t, target.Name)
	ctx := context.Background()
	_, err = buildTargetDefinitionChangePlan(ctx, target.Name, "")
	if err == nil {
		t.Fatal("expected collectable-orphan refusal")
	}
	classified := machine.Classify(err)
	if classified == nil || classified.Code != machine.ErrorOwnershipAmbiguous {
		t.Fatalf("refusal is not a typed machine error: %#v", err)
	}
	if classified.Next == "" || classified.Remediation != "operator_review" {
		t.Fatalf("refusal missing actionable next/remediation for automation: %#v", classified)
	}
}

// seedMismatchedUuidOrphan writes a deployment.json whose embedded DeploymentID
// does NOT equal the deployment directory name (a tampered / corrupt record):
// the folder is named "orphan-folder" while the record embeds a different UUID.
// This is the fixture shape that the round-2 decode-uuid-validation fix must
// reject: the record falls OUT of the collectable set even though the owner is
// still missing.
func seedMismatchedUuidOrphan(t *testing.T, target string) {
	t.Helper()
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", "orphan-folder")
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	intent, _ := json.Marshal(application.New("orphanapp", "dev", false, false, false))
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  "22222222-2222-4222-8222-222222222222",
			ApplicationID: application.MustNewApplicationID(),
			Target:        target,
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: "/nonexistent/repo",
			Manifest:   "/nonexistent/baseharbor.yaml",
		},
		Applied: deployment.AppliedDeployment{Intent: intent},
	}
	path := filepath.Join(depDir, "deployment.json")
	if err := os.WriteFile(path, mustJSON(t, record), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestTargetDeleteRefusalIgnoresUuidFolderMismatch verifies F2: a deployment
// record whose embedded DeploymentID does not match its directory name is NOT
// advertised as a collectable orphan, while the owner-missing guard still
// refuses the detach (fail-closed on the ownerless Target).
func TestTargetDeleteRefusalIgnoresUuidFolderMismatch(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(stateRoot, "runtime")); err != nil {
		t.Fatal("remove runtime owner for ownerless orphan", err)
	}
	seedMismatchedUuidOrphan(t, target.Name)

	orphans, err := deployment.CollectableOrphans(target.Name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on an ownerless mismatched orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("mismatched record must NOT be collectable, got %#v", orphans)
	}

	// Fail-closed: the detached ownerless Target still refuses (the owner-missing
	// guard fires on the directory regardless of the UUID/folder mismatch), just
	// not via the collectable-orphan cause — the mismatched record is no longer
	// advertised as collectable, so the plan falls through to the inventory
	// check, which still fails closed on the ownerless Target.
	ctx := context.Background()
	_, err = buildTargetDefinitionChangePlan(ctx, target.Name, "")
	if err == nil {
		t.Fatal("detach accepted an ownerless Target with a mismatched record without refusal")
	}
}

// seedBlankIdentityOrphan writes a deployment.json whose embedded DeploymentID or
// Target is missing or whitespace-only. The r1 decode-uuid fix only rejected
// non-empty mismatches, so a blank field slipped through and was advertised as
// collectable; F3 requires that a blank identity never be collectable.
func seedBlankIdentityOrphan(t *testing.T, target string) {
	t.Helper()
	deploymentID := "66666666-6666-4666-8666-666666666666"
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "deployments", deploymentID)
	if err := os.MkdirAll(depDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Blank DeploymentID and blank Target: neither proves its identity.
	intent, _ := json.Marshal(application.New("orphanapp", "dev", false, false, false))
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			DeploymentID:  "   ",
			ApplicationID: application.MustNewApplicationID(),
			Target:        "   ",
			Application:   "orphanapp",
			Environment:   "default",
		},
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: "/nonexistent/repo",
			Manifest:   "/nonexistent/baseharbor.yaml",
		},
		Applied: deployment.AppliedDeployment{Intent: intent},
	}
	path := filepath.Join(depDir, "deployment.json")
	if err := os.WriteFile(path, mustJSON(t, record), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestTargetDeleteRefusalIgnoresBlankIdentity verifies F3: a deployment record
// whose embedded DeploymentID or Target is whitespace-only is NOT advertised as
// a collectable orphan, while the owner-missing guard still refuses the detach
// (fail-closed on the ownerless Target).
func TestTargetDeleteRefusalIgnoresBlankIdentity(t *testing.T) {
	target := configureTestTarget(t)
	stateRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(stateRoot, "runtime")); err != nil {
		t.Fatal("remove runtime owner for ownerless orphan", err)
	}
	seedBlankIdentityOrphan(t, target.Name)

	orphans, err := deployment.CollectableOrphans(target.Name)
	if err != nil {
		t.Fatalf("CollectableOrphans errored on an ownerless blank-identity orphan: %v", err)
	}
	if len(orphans) != 0 {
		t.Fatalf("blank-identity record must NOT be collectable, got %#v", orphans)
	}

	// Fail-closed: the detached ownerless Target still refuses, and no record is
	// advertised as collectable under the collectable-orphan cause.
	ctx := context.Background()
	_, err = buildTargetDefinitionChangePlan(ctx, target.Name, "")
	if err == nil {
		t.Fatal("detach accepted an ownerless Target with a blank-identity record without refusal")
	}
	classified := machine.Classify(err)
	if classified != nil && classified.CauseCode == "target_definition_collectable_orphans" {
		t.Fatal("blank-identity record was advertised as a collectable orphan; it must not be")
	}
}
