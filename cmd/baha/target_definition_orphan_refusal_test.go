package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
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

// mustStateRoot resolves the Target state root for a test Target, failing fast
// on setup error so the call sites stay readable.
func mustStateRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	return root
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

// seedOwnerlessOrphanWithSource writes a deployment.json whose embedded
// DeploymentID equals the deployment directory name (a genuine collectable
// orphan) and whose source points at a real, existing directory so the record
// is healable once its Core owner is restored. The deployment directory is named
// with the SAME UUID the record embeds. This isolates the Core and source
// fixtures to this test. The source directory is removed on cleanup.
func seedOwnerlessOrphanWithSource(t *testing.T, target, deploymentID string) string {
	t.Helper()
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
			Repository: t.TempDir(),
			Manifest:   "/exists/baseharbor.yaml",
		},
		Applied: deployment.AppliedDeployment{Intent: intent},
	}
	if err := os.WriteFile(filepath.Join(depDir, "deployment.json"), mustJSON(t, record), 0600); err != nil {
		t.Fatal(err)
	}
	return deploymentID
}

// TestTargetDeleteHealPathReachesRecordWithOwnerRestored proves the documented
// recovery prerequisites actually allow the normal (healed) path to reach the
// record, execute `baha app destroy`, and remove it. This runs the real
// app-destroy lifecycle's ownership-verified cleanup + record removal against
// the Core-owner-restored Target, so the advertised healing path is executed,
// not merely asserted to be reachable. The Core and source fixtures are
// isolated to this test.
func TestTargetDeleteHealPathReachesRecordWithOwnerRestored(t *testing.T) {
	target := configureTestTarget(t) // registers the authoritative Core owner

	// Seed a real deployment record with a real, available source directory via
	// registerTestDeployment (the same seam `baha app apply` uses). The record
	// identity is the shared testDeploymentID with matching directory name, so
	// it is a genuine collectable orphan (decode-uuid == folder name) whose
	// source resolves on disk.
	manifest := application.New("orphanapp", "dev", false, false, false)
	seeded := registerTestDeployment(t, target, manifest, t.TempDir(), "/exists/baseharbor.yaml")
	id := seeded.Identity.DeploymentID

	// The record is loadable once its Core owner is present and its source is
	// available: this is the documented heal, via ListDeployments (the path
	// `baha app destroy` exercises).
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		t.Fatalf("record not reachable after owner restored: %v", err)
	}
	found := false
	for _, r := range records {
		if r.Identity.DeploymentID == id && r.Source.Repository != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("record with restored Core owner and available source was not reachable via ListDeployments (the app-destroy path)")
	}

	// Execute the app-destroy lifecycle's ownership-verified cleanup + record
	// removal. removeApplicationState verifies the application state is gone and
	// then deletes the deployment record (app_destroy_lifecycle.go:570-603) —
	// exactly the "baha app destroy removes the record after verified
	// destruction" the recovery text promises. A simple non-secret manifest
	// needs no live runtime, so no containers or engines are touched.
	store := application.Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: target.Name}
	e := applicationDestroyExecution{
		resolved: resolvedApplication{
			Target:             target,
			DeploymentIdentity: seeded.Identity,
			Manifest:           manifest,
			Store:              store,
			TargetStateRoot:    mustStateRoot(t, target.Name),
		},
		fullReset: true,
		confirmed: true,
		term:      cli.NewTerminal(context.Background(), io.Discard, io.Discard),
	}
	if err := e.removeApplicationState(); err != nil {
		t.Fatalf("app-destroy lifecycle did not remove the record: %v", err)
	}

	// The record is actually gone from the owner-resolved namespace.
	remaining, err := deployment.ListDeployments(target.Name)
	if err != nil {
		t.Fatalf("ListDeployments after destroy: %v", err)
	}
	for _, r := range remaining {
		if r.Identity.DeploymentID == id {
			t.Fatalf("app-destroy did not remove record %q; still present in list", id)
		}
	}
	// The deployment directory itself was removed (verified destruction).
	if _, statErr := os.Stat(filepath.Join(mustStateRoot(t, target.Name), "deployments", id)); !os.IsNotExist(statErr) {
		t.Fatalf("app-destroy left the deployment directory behind: %v", statErr)
	}

	// The Target definition and its Core installation are NOT destroyed by the
	// destroy: only the application/record is removed.
	cfg, cfgErr := deployment.LoadConfig()
	if cfgErr != nil {
		t.Fatalf("load config after destroy: %v", cfgErr)
	}
	if _, ok := cfg.Targets[target.Name]; !ok {
		t.Fatal("app-destroy removed the Target definition; it must remain attached")
	}
	if _, statErr := os.Stat(filepath.Join(mustStateRoot(t, target.Name), "runtime")); os.IsNotExist(statErr) {
		t.Fatal("app-destroy removed the Core runtime owner; it must remain")
	}

	// The refusal path (owner removed again) names the record, keeps the Target
	// attached, and does NOT offer the --from-recovery direction that cannot
	// heal this pre-detachment refusal. The heal above removed the only record,
	// so CollectableOrphans now finds nothing to refuse — which is itself the
	// proof that the heal succeeded. Seed a fresh collectable orphan to assert
	// the refusal contract on a still-attached record.
	freshID := seedOrphanDeploymentRecord(t, target.Name)
	if err := os.RemoveAll(filepath.Join(mustStateRoot(t, target.Name), "runtime")); err != nil {
		t.Fatal("remove runtime owner for ownerless orphan", err)
	}
	_, err = buildTargetDefinitionChangePlan(context.Background(), target.Name, "")
	if err == nil {
		t.Fatal("refusal did not fire for an attached collectable orphan")
	}
	if strings.Contains(err.Error(), "--from-recovery") {
		t.Fatal("refusal Next still offers 'target create --from-recovery', which cannot heal a pre-detachment refusal")
	}
	if !strings.Contains(err.Error(), freshID) {
		t.Fatalf("refusal did not name the attached record %q: %v", freshID, err)
	}
}

// TestTargetDefinitionDetachBlocksOnSharedTargetWideLock (case A) is the
// lock-specific production regression. It seeds a VALID Target state with NO
// collectable orphans, holds a local app lifecycle lock, and drives the actual
// production `deleteTarget` mutation. Because the orphan guard passes (no
// collectable orphan), the detach proceeds to
// lockExistingTargetDefinitionLifecycle and must contend on the SHARED
// target-wide lifecycle lock (coreinstallation.AcquireTargetWideLifecycleLock,
// carried by #1507). The detach must fail closed behind that lock — it must not
// complete while the local app lifecycle is held.
//
// This assertion is satisfied ONLY by contending on the target-wide lock: the
// orphan guard is explicitly excluded by the valid state, so the test fails if
// the production detach path stops acquiring AcquireTargetWideLifecycleLock (a
// per-operation lock would not contend with the held app lifecycle and the detach
// would succeed).
func TestTargetDefinitionDetachBlocksOnSharedTargetWideLock(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// Record the pristine Target config so the refusal can be shown to leave it
	// unchanged.
	cfgBefore, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config before detach: %v", err)
	}

	// Valid Target state with NO collectable orphans: the Core runtime owner is
	// present (ConfigureTestTarget registered it) and no ownerless deployment
	// record is seeded, so CollectableOrphans finds nothing and the orphan guard
	// passes — the detach proceeds to the shared target-wide lock below.

	// A local app holds its per-app lock through the production per-app path. This
	// is the only contention source in this case.
	appUnlock, aerr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-lived"},
		Manifest:           application.New("app-lived", "dev", false, false, false),
	}, false)
	if aerr != nil {
		t.Fatalf("local app acquire: %v", aerr)
	}
	t.Cleanup(appUnlock)

	// Verify up front that the state is genuinely orphan-free: the orphan guard
	// must NOT short-circuit this case (the lock contention is the only assertion).
	if orphans, perr := deployment.CollectableOrphans(target.Name); perr != nil || len(orphans) != 0 {
		t.Fatalf("case A requires a valid Target with no collectable orphans, got %d (err %v)", len(orphans), perr)
	}

	targetDefBefore, _ := json.Marshal(cfgBefore.Targets[target.Name])

	// The production detach path must fail closed behind the shared target-wide
	// lock while the local app lifecycle is held. It must NOT complete, and the
	// specific orphan-refusal outcome is explicitly NOT the assertion here — that
	// would let the test pass at the orphan guard instead of the lock.
	ctx := withAssumeYes(context.Background(), true)
	var out bytes.Buffer
	err = deleteTarget(ctx, []string{target.Name}, &out, &out)
	if err == nil {
		t.Fatal("production detach completed while a local app lifecycle was held; not routed through the shared target-wide lock")
	}
	classified := machine.Classify(err)
	if classified == nil {
		t.Fatalf("detach did not fail closed behind the shared lock: no classified error")
	}
	if classified.CauseCode == "target_definition_collectable_orphans" {
		t.Fatalf("case A asserted lock contention, not orphan refusal: %v", err)
	}

	// The Target definition must remain exactly as it was (a visible refusal, not
	// silent removal).
	cfgAfter, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config after refused detach: %v", err)
	}
	targetDefAfter, _ := json.Marshal(cfgAfter.Targets[target.Name])
	if string(targetDefBefore) != string(targetDefAfter) {
		t.Fatalf("refused detach mutated the Target definition: before=%s after=%s", targetDefBefore, targetDefAfter)
	}
	if _, ok := cfgAfter.Targets[target.Name]; !ok {
		t.Fatal("refused detach removed the configuration entry instead of refusing")
	}
}

// TestTargetDefinitionDetachRefusesOwnerlessRecord (case B) is the separate
// production-path orphan-refusal regression. It removes the Core runtime owner so
// a seeded deployment record becomes ownerless (a collectable orphan), then
// drives the production `deleteTarget` mutation. The orphan guard fires and the
// detach refuses, naming the attached record; the Target definition and orphan
// record remain unchanged.
func TestTargetDefinitionDetachRefusesOwnerlessRecord(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// Record the pristine config and seed the collectable orphan up front.
	cfgBefore, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config before detach: %v", err)
	}
	orphanID := seedOrphanDeploymentRecord(t, target.Name)

	// Remove the runtime owner so the seeded record is ownerless — the exact
	// orphan shape CollectableOrphans refuses.
	if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
		t.Fatal("remove runtime owner for ownerless orphan", err)
	}

	targetDefBefore, _ := json.Marshal(cfgBefore.Targets[target.Name])
	recordBefore, _ := os.ReadFile(filepath.Join(root, "deployments", orphanID, "deployment.json"))

	// The production detach path must refuse the collectable orphan, naming the
	// attached record, and must NOT complete or mutate config/the orphan record.
	ctx := withAssumeYes(context.Background(), true)
	var out bytes.Buffer
	err = deleteTarget(ctx, []string{target.Name}, &out, &out)
	if err == nil {
		t.Fatal("production detach completed with an ownerless collectable orphan attached")
	}
	classified := machine.Classify(err)
	if classified == nil || classified.CauseCode != "target_definition_collectable_orphans" {
		t.Fatalf("detach must refuse via the collectable-orphan guard, got: %v", err)
	}
	if !strings.Contains(err.Error(), orphanID) {
		t.Fatalf("orphan refusal must name the attached record %q: %v", orphanID, err)
	}

	// The Target definition must remain exactly as it was (a visible refusal, not
	// silent removal).
	cfgAfter, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load config after refused detach: %v", err)
	}
	targetDefAfter, _ := json.Marshal(cfgAfter.Targets[target.Name])
	if string(targetDefBefore) != string(targetDefAfter) {
		t.Fatalf("refused detach mutated the Target definition: before=%s after=%s", targetDefBefore, targetDefAfter)
	}
	if _, ok := cfgAfter.Targets[target.Name]; !ok {
		t.Fatal("refused detach removed the configuration entry instead of refusing")
	}

	// The attached orphan record must remain byte-for-byte unchanged (no pruning,
	// deletion, or owner rewrite).
	recordAfter, err := os.ReadFile(filepath.Join(root, "deployments", orphanID, "deployment.json"))
	if err != nil {
		t.Fatalf("orphan record missing after refused detach: %v", err)
	}
	if string(recordBefore) != string(recordAfter) {
		t.Fatalf("refused detach mutated the orphan record")
	}
}
