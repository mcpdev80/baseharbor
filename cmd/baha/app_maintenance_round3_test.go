package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// round3ReadMaintenanceHistory reads the durable maintenance history the way
// round-3 needs it: the retained entries in revision order. It is a thin
// production-path reader (MaintenanceHistoryFileName resolves the protected path),
// so a hidden history entry after a failed destroy is observable.
func round3ReadMaintenanceHistory(t *testing.T, stateRoot string) ([]application.MaintenanceAuthorityRecord, error) {
	t.Helper()
	path := filepath.Join(stateRoot, application.MaintenanceHistoryFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []application.MaintenanceAuthorityRecord
	if len(data) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// TestApplicationMaintenanceDestroyRollbackIsLoadbearing proves the round-3
// defect 1 rollback is load-bearing: with the rollback made a no-op, a failed
// destroy leaves the up-front window claiming a completed recovery point (red);
// the fix rolls it back and removes the tombstone so a failed destroy reports
// nothing as success (green). The production rollback is a var seam so the
// single-guard mutant can disable it without reaching into private package state.
func TestApplicationMaintenanceDestroyRollbackIsLoadbearing(t *testing.T) {
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

	// Green: the fixed path rolls back the up-front window and tombstone on owner
	// failure, so a failed destroy reports an error and leaves nothing behind.
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("a failed destroy was reported as success")
	}
	if _, found, lerr := application.LoadMaintenanceAuthority(resolved.stateRoot()); lerr != nil || found {
		t.Fatalf("a failed destroy left a durable window with the working rollback: found=%v", found)
	}
	tombPath, terr := maintenanceTombstonePath(resolved)
	if terr != nil {
		t.Fatalf("resolve tombstone path: %v", terr)
	}
	if _, statErr := os.Stat(tombPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a failed destroy left a tombstone with the working rollback: %v", statErr)
	}

	// Mutant: the rollback is a no-op. A failed destroy now leaves the up-front
	// window and tombstone claiming a completed recovery point.
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

// TestApplicationMaintenanceDestroyHistoryResidueAndRetry proves the round-3
// defect 2 fix: a failed destroy must remove both the current-window update and
// the destroy entry it appended to history, so a retry after recovery is not
// refused as a stale duplicate revision. Without the fix the failed destroy's
// history residue answers the retry as an already-recorded revision (red); with
// the fix the retry commits cleanly (green).
func TestApplicationMaintenanceDestroyHistoryResidueAndRetry(t *testing.T) {
	stateRoot := round2ProtectedState(t)

	// First durable operation (revision one) succeeds.
	if _, isNew, cerr := application.CommitMaintenanceAuthority(stateRoot,
		round2MaintenanceRecord("backup", "sha256:one", 1)); cerr != nil || !isNew {
		t.Fatalf("first durable commit: isNew=%t err=%v", isNew, cerr)
	}

	// A failed destroy (revision two) must leave neither an advanced current
	// window nor a history entry it appended. Inject one append failure through the
	// real transaction; the hook is restored after the failed attempt so the retry
	// appends successfully.
	original := application.AppendMaintenanceHistoryHook
	t.Cleanup(func() { application.AppendMaintenanceHistoryHook = original })
	application.AppendMaintenanceHistoryHook = func(stateRoot string, submitted application.MaintenanceAuthorityRecord) error {
		return application.ErrMaintenanceHistoryAppendFailed
	}
	application.AppendMaintenanceHistoryHookRolledBack = false
	destroy := round2MaintenanceRecord("destroy", "sha256:two", 2)
	_, _, cerr := application.CommitMaintenanceAuthority(stateRoot, destroy)
	if cerr == nil {
		t.Fatal("a failed destroy was reported as success")
	}
	if !errors.Is(cerr, application.ErrMaintenanceHistoryAppendFailed) {
		t.Fatalf("the failure was not the named fail-closed error: %v", cerr)
	}

	// No partial state is reported as success: the current window is not advanced
	// past the missing history entry.
	current, found, lerr := application.LoadMaintenanceAuthority(stateRoot)
	if lerr != nil {
		t.Fatalf("reload after failed destroy: %v", lerr)
	}
	if found && current.Revision != 1 {
		t.Fatalf("the current window advanced past a missing history entry: rev=%d", current.Revision)
	}

	// The failed destroy's history entry is gone: history holds only revision one.
	records, herr := round3ReadMaintenanceHistory(t, stateRoot)
	if herr != nil {
		t.Fatalf("read history after failed destroy: %v", herr)
	}
	if len(records) != 1 || records[0].Revision != 1 {
		t.Fatalf("a failed destroy left history residue: %+v", records)
	}

	// The retry after recovery commits cleanly and answers its own revision.
	application.AppendMaintenanceHistoryHook = original
	if _, _, cerr := application.CommitMaintenanceAuthority(stateRoot, destroy); cerr != nil {
		t.Fatalf("a retry after the repaired destroy failed: %v", cerr)
	}
	if _, found, rerr := application.LoadMaintenanceRevisionForTest(stateRoot, 2); rerr != nil || !found {
		t.Fatalf("the retry did not answer the committed destroy revision: found=%v err=%v", found, rerr)
	}
	if _, found, rerr := application.LoadMaintenanceRevisionForTest(stateRoot, 1); rerr != nil || !found {
		t.Fatalf("the earlier revision is unanswerable after the destroy retry: found=%v err=%v", found, rerr)
	}
}

var _ = json.MarshalIndent
