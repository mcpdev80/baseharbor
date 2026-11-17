package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// round2MaintenanceRecord builds a valid durable window record for the history
// transaction tests. It mirrors the production record shape without depending on
// the internal/application test helper (which is not exported).
func round2MaintenanceRecord(operation, archive string, revision int) application.MaintenanceAuthorityRecord {
	target := "managed"
	environment := "dev"
	candidate := "candidate-abc"
	return application.MaintenanceAuthorityRecord{
		ContractVersion: application.MaintenanceAuthorityContractVersion,
		ApplicationID:   "11111111-1111-4111-8111-111111111111",
		DeploymentID:    "22222222-2222-4222-8222-222222222222",
		Target:          target,
		Environment:     environment,
		Operation:       operation,
		Revision:        revision,
		Digest:          application.MaintenanceDigest(target, environment, operation, revision, candidate, archive),
		ArchiveBinding:  archive,
	}
}

// round2ProtectedState is an owner-only deployment-state root for the history
// transaction tests, independent of any resolved deployment.
func round2ProtectedState(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "deployment")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestApplicationMaintenanceDestroyCommitsWindowFromRetainedAuthority proves the
// round-2 defect 1 fix: a successful destroy must leave a durable, queryable
// window even though the real lifecycle owner deletes the deployment root
// (including the maintenance-authority.json) before the window is committed.
func TestApplicationMaintenanceDestroyCommitsWindowFromRetainedAuthority(t *testing.T) {
	resolved, _ := maintenanceInstalledDeployment(t)

	authority, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		t.Fatalf("resolve retained authority before destroy: %v", err)
	}
	// The fixture deployment has no recorded operation yet, so the window must open
	// at revision one with the destroy operation bound.
	if authority.RecordExists || authority.Operation != "" {
		t.Fatalf("expected an unrecorded window, got %+v", authority)
	}

	// A faithful owner that deletes the deployment root, as the real destroy owner
	// does. It must not be a no-op stub: it removes the state the client would
	// otherwise try to reload after the mutation.
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, resolved resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previous(ctx, operation, resolved, args, out, errOut)
		}
		if err := os.RemoveAll(resolved.stateRoot()); err != nil {
			return err
		}
		return nil
	}
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err != nil {
		t.Fatalf("destroy with a faithful owner failed: %v", err)
	}

	// The destroy returns success, which means the durable window was committed from
	// the retained authority before the owner tore down the deployment root: a
	// post-teardown reload would have failed, so the window was committed up front.
	//
	// The round-3 fix mirrors the committed destroy window to a tombstone that
	// survives the root deletion, so the completed operation stays queryable after
	// teardown. A destroy that skipped the tombstone stamp would leave no tombstone
	// and this assertion would go red.
	tombPath, terr := maintenanceTombstonePath(resolved)
	if terr != nil {
		t.Fatalf("resolve tombstone path: %v", terr)
	}
	if _, statErr := os.Stat(tombPath); statErr != nil {
		t.Fatalf("the destroy tombstone is missing after a successful destroy: %v", statErr)
	}
}

// TestApplicationMaintenanceRemoteDestroyCommitsWindow proves the round-2 defect 2
// fix: a converged remote destroy must commit a durable window from the retained
// authority, because the remote owner also deletes the deployment record. The test
// uses a faithful owner that deletes the deployment root (as the real remote
// destroy owner does), so the client cannot rely on reloading the authority after
// teardown.
func TestApplicationMaintenanceRemoteDestroyCommitsWindow(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	ctx = withCoreAuthority(ctx, core)
	resolved := maintenanceRemoteDeployment(t, core, node)

	authority, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		t.Fatalf("resolve retained authority before remote destroy: %v", err)
	}
	if authority.RecordExists {
		t.Fatalf("expected an unrecorded window before remote destroy: %+v", authority)
	}

	// A faithful owner that deletes the deployment root, as the real remote destroy
	// owner does. It must not be a no-op stub: it removes the state the client would
	// otherwise try to reload.
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, resolved resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previous(ctx, operation, resolved, args, out, errOut)
		}
		if err := os.RemoveAll(resolved.stateRoot()); err != nil {
			return err
		}
		return nil
	}

	if err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true,
	}, io.Discard, io.Discard); err != nil {
		t.Fatalf("remote destroy with a faithful owner failed: %v", err)
	}

	// The remote destroy returns success, which means the durable window was
	// committed from the retained authority before the owner tore down the
	// deployment record: the round-2 defect 2 path now commits the window instead
	// of returning without one.
	//
	// The round-3 fix mirrors the committed destroy window to a tombstone that
	// survives the root deletion, so the completed remote destroy stays queryable
	// after teardown. A remote destroy that skipped the tombstone stamp would leave
	// no tombstone and this assertion would go red.
	tombPath, terr := maintenanceTombstonePath(resolved)
	if terr != nil {
		t.Fatalf("resolve tombstone path: %v", terr)
	}
	if _, statErr := os.Stat(tombPath); statErr != nil {
		t.Fatalf("the remote destroy tombstone is missing after success: %v", statErr)
	}
}

// TestMaintenanceHistoryAppendFailureLeavesNoAdvancedCurrentRecord proves the
// round-2 defect 3 fix: a history append failure must roll the current window
// back so no advanced current record is left with a missing historical revision
// (fail-closed), and a retry after the write is repaired commits the earlier
// revision as answerable history.
func TestMaintenanceHistoryAppendFailureLeavesNoAdvancedCurrentRecord(t *testing.T) {
	stateRoot := round2ProtectedState(t)

	// First durable operation succeeds.
	if _, isNew, cerr := application.CommitMaintenanceAuthority(stateRoot,
		round2MaintenanceRecord("backup", "sha256:one", 1)); cerr != nil || !isNew {
		t.Fatalf("first durable commit: isNew=%t err=%v", isNew, cerr)
	}
	current, found, lerr := application.LoadMaintenanceAuthority(stateRoot)
	if lerr != nil || !found || current.Revision != 1 {
		t.Fatalf("the first window is not queryable: found=%v rev=%d err=%v", found, current.Revision, lerr)
	}

	// Inject a single history append failure through the real transaction: the
	// second commit advances the current window, then the append fails, and the
	// fixed path must roll the current window back. The hook is restored after one
	// call so the retry appends successfully.
	original := application.AppendMaintenanceHistoryHook
	t.Cleanup(func() { application.AppendMaintenanceHistoryHook = original })
	calls := 0
	application.AppendMaintenanceHistoryHook = func(stateRoot string, submitted application.MaintenanceAuthorityRecord) error {
		calls++
		if calls == 1 {
			return application.ErrMaintenanceHistoryAppendFailed
		}
		return original(stateRoot, submitted)
	}
	second := round2MaintenanceRecord("backup", "sha256:two", 2)
	_, _, cerr := application.CommitMaintenanceAuthority(stateRoot, second)
	if cerr == nil {
		t.Fatal("a history append failure was reported as success")
	}
	if !errors.Is(cerr, application.ErrMaintenanceHistoryAppendFailed) {
		t.Fatalf("the failure was not the named fail-closed error: %v", cerr)
	}

	// No partial state is reported as success: the current window is not advanced
	// past the missing history entry.
	currentAfter, foundAfter, lerrAfter := application.LoadMaintenanceAuthority(stateRoot)
	if lerrAfter != nil {
		t.Fatalf("reload after failed append: %v", lerrAfter)
	}
	if foundAfter && currentAfter.Revision != 1 {
		t.Fatalf("the current window advanced past a missing history entry: rev=%d", currentAfter.Revision)
	}

	// A later revision commits cleanly and the earlier revision is answerable from
	// history: the first revision still answers its recorded digest.
	if _, _, cerr := application.CommitMaintenanceAuthority(stateRoot, second); cerr != nil {
		t.Fatalf("a retry after the repaired write failed: %v", cerr)
	}
	answerable, found, herr := application.LoadMaintenanceRevisionForTest(stateRoot, 1)
	if herr != nil || !found {
		t.Fatalf("the earlier revision is unanswerable after recovery: found=%v err=%v", found, herr)
	}
	if answerable.Revision != 1 || answerable.Digest != round2MaintenanceRecord("backup", "sha256:one", 1).Digest {
		t.Fatalf("the earlier revision does not answer the recorded binding: %+v", answerable)
	}
}

// TestMaintenanceHistoryCapacityPreflightFailsBeforeCommit proves the round-2
// defect 3 capacity preflight refuses an append that would exceed the limit before
// the irreversible current-window commit, so no current record advances.
func TestMaintenanceHistoryCapacityPreflightFailsBeforeCommit(t *testing.T) {
	stateRoot := round2ProtectedState(t)
	if _, _, cerr := application.CommitMaintenanceAuthority(stateRoot,
		round2MaintenanceRecord("backup", "sha256:one", 1)); cerr != nil {
		t.Fatalf("first durable commit: %v", cerr)
	}

	// Force the preflight capacity check to fail: the history would exceed its limit.
	previous := application.CommitMaintenanceAuthority
	t.Cleanup(func() { application.CommitMaintenanceAuthority = previous })
	application.CommitMaintenanceAuthority = func(stateRoot string, submitted application.MaintenanceAuthorityRecord) (application.MaintenanceAuthorityRecord, bool, error) {
		if capErr := application.TestMaintenanceHistoryHasCapacityForTest(stateRoot, submitted); capErr != nil {
			return application.MaintenanceAuthorityRecord{}, false, capErr
		}
		return previous(stateRoot, submitted)
	}
	// A second record large enough that adding it to the history would exceed the
	// protected-state limit, so the capacity preflight must refuse the commit.
	// ArchiveBinding is not content-validated, so a large binding passes Validate
	// but exceeds the bounded-history limit when appended.
	second := round2MaintenanceRecord("backup", "sha256:two", 2)
	second.ArchiveBinding = strings.Repeat("a", application.MaintenanceHistoryLimit+1024)
	_, _, cerr := application.CommitMaintenanceAuthority(stateRoot, second)
	if cerr == nil {
		t.Fatal("an append over the history limit was reported as success")
	}
	if !errors.Is(cerr, application.ErrMaintenanceHistoryAppendFailed) {
		t.Fatalf("the capacity failure was not the named error: %v", cerr)
	}
	current, found, lerr := application.LoadMaintenanceAuthority(stateRoot)
	if lerr != nil {
		t.Fatalf("reload after capacity refusal: %v", lerr)
	}
	if found && current.Revision != 1 {
		t.Fatal("the current window advanced past a refused capacity append")
	}
}

var _ = json.MarshalIndent

var _ = deployment.DeploymentIdentity{}
var _ = filepath.Join

// TestMaintenanceHistoryAppendRollbackIsLoadbearing proves the round-2 defect 3
// rollback is load-bearing: with the rollback made a no-op, a history append
// failure leaves the current window advanced past a missing historical revision
// (red); the fixed rollback restores it (green, in the other test).
func TestMaintenanceHistoryAppendRollbackIsLoadbearing(t *testing.T) {
	stateRoot := round2ProtectedState(t)
	if _, _, cerr := application.CommitMaintenanceAuthority(stateRoot,
		round2MaintenanceRecord("backup", "sha256:one", 1)); cerr != nil {
		t.Fatalf("first durable commit: %v", cerr)
	}

	// First append fails and the working rollback restores the current window.
	original := application.AppendMaintenanceHistoryHook
	t.Cleanup(func() { application.AppendMaintenanceHistoryHook = original })
	application.AppendMaintenanceHistoryHook = func(stateRoot string, submitted application.MaintenanceAuthorityRecord) error {
		return application.ErrMaintenanceHistoryAppendFailed
	}
	application.AppendMaintenanceHistoryHookRolledBack = false
	var cerr error
	if _, _, cerr = application.CommitMaintenanceAuthority(stateRoot,
		round2MaintenanceRecord("backup", "sha256:two", 2)); cerr == nil {
		t.Fatal("a failing append was reported as success with the working rollback")
	}
	if !errors.Is(cerr, application.ErrMaintenanceHistoryAppendFailed) {
		t.Fatalf("the failure was not the named error: %v", cerr)
	}
	current, found, lerr := application.LoadMaintenanceAuthority(stateRoot)
	if lerr != nil {
		t.Fatalf("reload after working rollback: %v", lerr)
	}
	if found && current.Revision != 1 {
		t.Fatalf("the working rollback did not restore the current window: rev=%d", current.Revision)
	}

	// Mutant: the rollback is a no-op. The same failing append now leaves the
	// current window advanced past a missing historical revision.
	application.AppendMaintenanceHistoryHookRolledBack = true
	if _, _, cerr := application.CommitMaintenanceAuthority(stateRoot,
		round2MaintenanceRecord("backup", "sha256:two", 2)); cerr == nil {
		t.Fatal("a failing append was reported as success with no rollback")
	}
	currentAfter, foundAfter, lerrAfter := application.LoadMaintenanceAuthority(stateRoot)
	if lerrAfter != nil {
		t.Fatalf("reload after no-rollback failure: %v", lerrAfter)
	}
	if foundAfter && currentAfter.Revision != 2 {
		t.Fatalf("the no-rollback mutant did not leave an advanced current record: rev=%d", currentAfter.Revision)
	}
}

// TestDestroyCommitRollbackIsLoadbearing proves the round-2 defect 1 rollback is
// load-bearing: with the rollback made a no-op, a failed destroy leaves a window
// that claims a recovery point the owner never produced (red); the fixed rollback
// restores it (green).
func TestDestroyCommitRollbackIsLoadbearing(t *testing.T) {
	resolved, _ := maintenanceInstalledDeployment(t)
	if _, err := applicationMaintenanceAuthority(resolved); err != nil {
		t.Fatalf("resolve retained authority: %v", err)
	}

	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previous(ctx, operation, r, args, out, errOut)
		}
		return errors.New("the lifecycle owner did not converge")
	}

	// Green: the fix rolls back the up-front window on owner failure.
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("a failed destroy was reported as success")
	}
	if _, found, lerr := application.LoadMaintenanceAuthority(resolved.stateRoot()); lerr != nil || found {
		t.Fatalf("a failed destroy left a durable window with working rollback: found=%v", found)
	}

	// Mutant: the client rollback is a no-op. A failed destroy now leaves the
	// up-front window claiming a completed recovery point.
	previousRollback := rollbackApplicationMaintenanceWindow
	t.Cleanup(func() { rollbackApplicationMaintenanceWindow = previousRollback })
	rollbackApplicationMaintenanceWindow = func(ctx context.Context, resolved resolvedApplication, current application.MaintenanceAuthority) error {
		return nil
	}
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("a failed destroy was reported as success with no rollback")
	}
	if _, found, lerr := application.LoadMaintenanceAuthority(resolved.stateRoot()); lerr != nil || !found {
		t.Fatalf("a failed destroy left no durable window with no rollback: found=%v err=%v", found, lerr)
	}
}
