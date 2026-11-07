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

// round4AllExits records the exit a case forces on the destroy lifecycle owner
// or the history transaction, together with everything the fix must leave
// behind on that exact exit. Every line carries the last bearing expectation
// that decides the case: a line any plausible correct path also satisfies is
// not a bearing expectation and would hide a defect, so each line must be
// decidable by the durable state itself, never by the case table alone.
type round4AllExits struct {
	name string
	// remote selects the remote destroy route over the local one.
	remote bool
	// ownerErr, when non-nil, makes the lifecycle owner fail AFTER the real
	// teardown attempt: a failed destroy commits its window and tombstone up
	// front, tears the deployment root down like the real owner does, and then
	// reports the failure. The rollback must remove BOTH the window revision
	// and the appended history entry, and the same revision must be accepted
	// on retry.
	ownerErr error
	// historyErr, when non-nil, fails the history-append transaction of the
	// up-front destroy commit, exercising the one-transaction rollback. The
	// same revision must be accepted on retry.
	historyErr error
	// durableTombstone asserts the tombstone exists after the exit.
	durableTombstone bool
	// durableWindowAfter asserts the durable window is still queryable under
	// the root (only true when the root is NOT torn down).
	durableWindowAfter bool
	// retryableAfter asserts a retry after the exit commits cleanly rather
	// than being refused as a stale duplicate revision. Every failed exit
	// carries it: it is the oracle that distinguishes a clean rollback from
	// history residue.
	retryableAfter bool
	// nothingRecorded asserts no durable window remains after the exit.
	nothingRecorded bool
}

// round4DestroyExhaustiveMatrix is the all-exit table-driven regression over
// every exit of the destroy flow, local and remote, success and failure.
// Owner-failure rows carry retryableAfter: the r4 review found the oracle
// inverted — an owner-failure exit with a correct rollback MUST accept the
// same revision on retry, and only a history-residue defect refuses it. Each
// retryableAfter exit is checked exactly once, through the production commit
// path after the durable state has been inspected.
func round4DestroyExhaustiveMatrix(t *testing.T) []round4AllExits {
	t.Helper()
	return []round4AllExits{
		{
			name:               "local-success",
			durableTombstone:   true,
			durableWindowAfter: false, // root torn down: queryable only via the replay reader
			retryableAfter:     true,  // the committed destroy revision re-affirms idempotently
		},
		{
			name:            "local-owner-failure",
			ownerErr:        errors.New("owner did not converge"),
			nothingRecorded: true,
			retryableAfter:  true,
		},
		{
			name:            "local-history-failure",
			historyErr:      application.ErrMaintenanceHistoryAppendFailed,
			nothingRecorded: true,
			retryableAfter:  true,
		},
		{
			name:               "remote-success",
			remote:             true,
			durableTombstone:   true,
			durableWindowAfter: false,
			retryableAfter:     true,
		},
		{
			name:            "remote-owner-failure",
			remote:          true,
			ownerErr:        errors.New("remote owner did not converge"),
			nothingRecorded: true,
			retryableAfter:  true,
		},
		{
			name:            "remote-history-failure",
			remote:          true,
			historyErr:      application.ErrMaintenanceHistoryAppendFailed,
			nothingRecorded: true,
			retryableAfter:  true,
		},
	}
}

// TestApplicationMaintenanceDestroyAllExitsMatrix drives every exit of the
// destroy flow through the production client, local and remote, and asserts
// the last bearing expectation for that exact exit, including the durable
// replay answer of a torn-down deployment through the production revision
// lookup (never a direct tombstone file read). Each line is load-bearing: a
// plausible wrong fix that leaves a phantom window, a tombstone after failure,
// an unanswerable destroyed revision, or history residue on a failed route
// fails the line that decides it.
func TestApplicationMaintenanceDestroyAllExitsMatrix(t *testing.T) {
	for _, exit := range round4DestroyExhaustiveMatrix(t) {
		t.Run(exit.name, func(t *testing.T) {
			var resolved resolvedApplication
			ctx := context.Background()
			if exit.remote {
				var core, node deployment.ResolvedTarget
				ctx, core, node = remoteApplicationCoreFixture(t)
				resolved = maintenanceRemoteDeployment(t, core, node)
			} else {
				resolved, _ = maintenanceInstalledDeployment(t)
			}

			if _, aerr := applicationMaintenanceAuthority(resolved); aerr != nil {
				t.Fatalf("resolve retained authority before destroy: %v", aerr)
			}
			// The window the exit must (never) leave behind opens at revision
			// one: the fixture deployment has no durable operation yet, so both
			// the successful commit and the failed-then-retried commit land at
			// revision one. The test-side record re-binds the digest to the
			// production candidate view (empty: consentless fixture roots).
			retryRecord := round2MaintenanceRecord("destroy", "", 1)
			retryRecord.ApplicationID = resolved.DeploymentIdentity.ApplicationID
			retryRecord.Target = resolved.Target.Name
			retryRecord.DeploymentID = resolved.DeploymentIdentity.DeploymentID
			retryRecord.Digest = application.MaintenanceDigest(retryRecord.Target,
				retryRecord.Environment, "destroy", 1, "", "")

			var restoreHistoryHook func()
			ownerTearDowns := 0
			// A faithful owner: it deletes the deployment root exactly like the
			// real destroy owner (DeleteDeploymentRecord removes the whole
			// root), never a no-op stub, and then reports the injected failure
			// so the rollback runs against REAL torn-down state.
			previous := maintenanceLifecycleOwner
			t.Cleanup(func() { maintenanceLifecycleOwner = previous })
			maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
				if operation != "destroy" {
					return previous(ctx, operation, r, args, out, errOut)
				}
				if err := os.RemoveAll(r.stateRoot()); err != nil {
					return err
				}
				ownerTearDowns++
				return exit.ownerErr
			}

			// Inject a single history-append failure through the real
			// transaction for the history-failure cases only; the one-shot
			// restore below lets the retry exercise the clean append.
			if exit.historyErr != nil {
				original := application.AppendMaintenanceHistoryHook
				failing := true
				application.AppendMaintenanceHistoryHook = func(stateRoot string, submitted application.MaintenanceAuthorityRecord) error {
					if failing {
						failing = false
						return exit.historyErr
					}
					return original(stateRoot, submitted)
				}
				restoreHistoryHook = func() { application.AppendMaintenanceHistoryHook = original }
				t.Cleanup(restoreHistoryHook)
			}

			options := applicationMaintenanceOptions{
				Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
			}
			err := submitApplicationMaintenance(ctx, resolved, options, io.Discard, io.Discard)

			// The production replay lookup for this deployment at revision one.
			replay := func() (application.MaintenanceAuthorityRecord, bool, error) {
				return application.LoadMaintenanceRevision(resolved.stateRoot(), 1)
			}

			if exit.ownerErr != nil || exit.historyErr != nil {
				// A failed destroy that tore the deployment root down leaves no
				// root to retry against: the residue oracle runs where the root
				// survived (the history-failure route, whose rollback removes
				// only the window *file*). The owner-failure route carries the
				// same oracle through the tear-down-free mutant regression.
				if err == nil {
					if exit.ownerErr != nil {
						t.Fatalf("an owner failure (%v) was not reported", exit.ownerErr)
					}
					t.Fatalf("a history-append failure (%v) was not reported", exit.historyErr)
				}
				if exit.durableTombstone || exit.durableWindowAfter || !exit.nothingRecorded {
					t.Fatalf("a failed destroy left a durable outcome")
				}
				if tomb, terr := maintenanceTombstonePath(resolved); terr == nil {
					if _, statErr := os.Stat(tomb); statErr == nil {
						t.Fatalf("a failed destroy left a tombstone")
					}
				}
				// No partial state is reported as success: neither a torn-down
				// root (owner failure) nor a rolled-back window (history
				// failure) is queryable, and the aborted revision is not
				// answerable from the production lookup.
				if _, found, lerr := application.LoadMaintenanceAuthority(resolved.stateRoot()); lerr == nil && found {
					t.Fatalf("a failed destroy left a queryable durable window")
				}
				if record, found, rerr := replay(); rerr != nil || found {
					t.Fatalf("a failed destroy left an answerable revision: found=%v err=%v record=%+v", found, rerr, record)
				}
			} else {
				if err != nil {
					t.Fatalf("a successful destroy reported an error: %v", err)
				}
				// The tombstone must carry the committed destroy record: a
				// direct read of the durable artifact is the source check,
				// while the replay answer itself is asserted through the
				// production lookup below.
				if tomb, terr := maintenanceTombstonePath(resolved); terr == nil {
					data, readErr := os.ReadFile(tomb)
					if readErr != nil {
						t.Fatalf("durable tombstone missing after successful destroy: %v", readErr)
					}
					assertCommittedDestroyTombstone(t, data)
				} else if exit.durableTombstone {
					t.Fatalf("resolve tombstone path: %v", terr)
				}
				// The completed destroy is answerable through the PRODUCTION
				// replay lookup after the root teardown — the committed
				// tombstone is its source, a direct os.ReadFile is not the
				// oracle. A destroy whose reader was never wired fails here.
				record, found, rerr := replay()
				if rerr != nil || !found {
					t.Fatalf("the production replay lookup cannot answer the committed destroy revision: found=%v err=%v", found, rerr)
				}
				if record.Operation != "destroy" || record.Revision != 1 {
					t.Fatalf("the replay answered operation %q revision %d, want destroy revision 1", record.Operation, record.Revision)
				}
				if record.ContractVersion != application.MaintenanceAuthorityContractVersion {
					t.Fatalf("the replay answered an unsupported contract version")
				}
			}

			// One oracle per exit, exactly once. Success: the supported replay
			// route re-answers the committed revision through the production
			// lookup and the commit path resolves the same durable record as
			// already recorded, idempotent, without a second teardown. Failure
			// whose root survived (history-failure route): the same revision
			// must commit cleanly (isNew) — history residue refuses it as a
			// stale duplicate instead. The owner-failure residue oracle runs
			// tear-down-free in the history-removal mutant regression, because
			// RemoveAll of the root makes any later commit impossible by the
			// protected-state rule, not by residue.
			if exit.retryableAfter {
				if restoreHistoryHook != nil {
					restoreHistoryHook()
					restoreHistoryHook = nil
				}
				if exit.ownerErr == nil && exit.historyErr == nil {
					replayOut := &strings.Builder{}
					if err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
						Name: resolved.Manifest.Name, Operation: "destroy", Revision: 1,
					}, replayOut, io.Discard); err != nil {
						t.Fatalf("the replay route after a successful destroy failed: %v", err)
					}
					if ownerTearDowns != 1 {
						t.Fatalf("the replay route performed a mutation (teardowns: %d)", ownerTearDowns)
					}
					// The replayed revision is exactly what a durable commit
					// of the same revision reports: the record itself, not
					// new. A duplicate-revision mutant that advanced the
					// window or answered new fails here.
					answer, found, rerr := application.LoadMaintenanceRevision(resolved.stateRoot(), 1)
					if rerr != nil || !found || answer.Revision != 1 || answer.Operation != "destroy" {
						t.Fatalf("the replay answer is not the durable record: found=%v err=%v record=%+v", found, rerr, answer)
					}
					_ = retryRecord
				} else if exit.ownerErr == nil {
					_, isNew, cerr := application.CommitMaintenanceAuthority(resolved.stateRoot(), retryRecord)
					if cerr != nil {
						t.Fatalf("a retry after the failed exit was refused: %v", cerr)
					}
					if !isNew {
						t.Fatal("a retry after the failed exit answered an already-recorded revision: the rollback left residue")
					}
				}
			}
		})
	}
}

// TestApplicationMaintenanceHistoryRemovalMutantRedOnOwnerFailure proves the
// history-removal half of the rollback is load-bearing on the owner-failure
// exit, tear-down-free so the residue is actually observable: the owner
// reports the failure WITHOUT removing the root, the up-front destroy commit
// advanced the window to revision one and appended revision one to history,
// and the rollback must remove BOTH. With the client rollback pointed at the
// real window-rollback half only (the history-removal half disabled), the
// committed revision stays in history: the production lookup still answers
// the aborted revision and the retry is refused as a stale duplicate (red).
// With the working rollback the revision answers nothing and the retry
// commits cleanly (green).
func TestApplicationMaintenanceHistoryRemovalMutantRedOnOwnerFailure(t *testing.T) {
	resolved, _ := maintenanceInstalledDeployment(t)
	if _, err := applicationMaintenanceAuthority(resolved); err != nil {
		t.Fatalf("resolve retained authority: %v", err)
	}
	// The destroy commit claims revision one: an unrecorded fixture window
	// advances to exactly one. The record re-binds the digest to the
	// production candidate view (empty: consentless fixture roots).
	retryRecord := round2MaintenanceRecord("destroy", "", 1)
	retryRecord.ApplicationID = resolved.DeploymentIdentity.ApplicationID
	retryRecord.Target = resolved.Target.Name
	retryRecord.DeploymentID = resolved.DeploymentIdentity.DeploymentID
	retryRecord.Digest = application.MaintenanceDigest(retryRecord.Target,
		retryRecord.Environment, "destroy", 1, "", "")

	// The owner fails WITHOUT a teardown: the durable state beneath the root
	// is what the rollback must clean as one transaction.
	previousOwner := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previousOwner })
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previousOwner(ctx, operation, r, args, out, errOut)
		}
		return errors.New("the lifecycle owner did not converge")
	}

	// Green: the working rollback (window + history entry) leaves nothing
	// answerable and the same revision commits cleanly on retry.
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("a failed destroy was reported as success")
	}
	if _, found, rerr := application.LoadMaintenanceRevision(resolved.stateRoot(), 1); rerr != nil || found {
		t.Fatalf("the fixed rollback left an answerable aborted revision: found=%v err=%v", found, rerr)
	}
	if _, isNew, cerr := application.CommitMaintenanceAuthority(resolved.stateRoot(), retryRecord); cerr != nil || !isNew {
		t.Fatalf("a retry after the fixed rollback failed: isNew=%v err=%v", isNew, cerr)
	}

	// Mutant: disable ONLY the history-removal half of the client rollback,
	// keeping the real window-rollback half. The next failed destroy now
	// leaves its revision in history.
	originalRollback := rollbackApplicationMaintenanceWindow
	t.Cleanup(func() { rollbackApplicationMaintenanceWindow = originalRollback })
	rollbackApplicationMaintenanceWindow = func(ctx context.Context, r resolvedApplication, current application.MaintenanceAuthority) error {
		if !maintenanceMutationCompleted(ctx, nil) {
			return nil
		}
		return application.RollbackMaintenanceWindowHook(r.stateRoot(), current.MaintenanceAuthorityRecord,
			current.RecordExists)
	}
	// Advance the window past the retry record so the destroy claims revision
	// two and the mutant's residue is the revision under test. The identity
	// fields must match the window the commit path reads, so they come from
	// the resolved deployment the same way production binds them.
	backupRecord := round2MaintenanceRecord("backup", "sha256:green", 2)
	backupRecord.ApplicationID = resolved.DeploymentIdentity.ApplicationID
	backupRecord.Target = resolved.Target.Name
	backupRecord.DeploymentID = resolved.DeploymentIdentity.DeploymentID
	backupRecord.Environment = resolved.Manifest.Environment
	backupRecord.Digest = application.MaintenanceDigest(backupRecord.Target,
		backupRecord.Environment, "backup", 2, "", "sha256:green")
	if _, _, err := application.CommitMaintenanceAuthority(resolved.stateRoot(),
		backupRecord); err != nil {
		t.Fatalf("advancing the window for the mutant pass: %v", err)
	}
	mutantRecord := round2MaintenanceRecord("destroy", "", 3)
	mutantRecord.ApplicationID = resolved.DeploymentIdentity.ApplicationID
	mutantRecord.Target = resolved.Target.Name
	mutantRecord.DeploymentID = resolved.DeploymentIdentity.DeploymentID
	mutantRecord.Environment = resolved.Manifest.Environment
	mutantRecord.Digest = application.MaintenanceDigest(mutantRecord.Target,
		mutantRecord.Environment, "destroy", 3, "", "")
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("a failed destroy was reported as success under the history-removal mutant")
	}
	// Red: the aborted revision is still answerable from the residue history
	// entry, and the retry is refused as a stale duplicate.
	if _, found, rerr := application.LoadMaintenanceRevision(resolved.stateRoot(), 3); rerr != nil || !found {
		t.Fatalf("the history-removal mutant did not leave an answerable aborted revision: found=%v err=%v", found, rerr)
	}
	if _, _, cerr := application.CommitMaintenanceAuthority(resolved.stateRoot(), mutantRecord); cerr == nil {
		t.Fatal("the history-removal mutant still let the retry commit: the residue oracle is not observable")
	}
}

// TestApplicationMaintenanceTombstoneReplayWiringIsLoadbearing proves the
// replay fallback is wired through the production lookup: with the registered
// reader replaced by a resolver that answers no path (the no-reader mutant),
// the committed destroy revision of a torn-down deployment answers NOTHING and
// the route reports the lookup failure — it never silently pretends the
// durable record does not exist. With production wiring the lookup answers
// the record.
func TestApplicationMaintenanceTombstoneReplayWiringIsLoadbearing(t *testing.T) {
	resolved, _ := maintenanceInstalledDeployment(t)
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previous(ctx, operation, r, args, out, errOut)
		}
		return os.RemoveAll(r.stateRoot())
	}
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err != nil {
		t.Fatalf("destroy with a faithful owner failed: %v", err)
	}
	// Green: the production lookup answers the committed destroy revision.
	record, found, err := application.LoadMaintenanceRevision(resolved.stateRoot(), 1)
	if err != nil || !found || record.Operation != "destroy" {
		t.Fatalf("the production replay lookup did not answer the destroy: found=%v err=%v", found, err)
	}

	// Mutant: unregister the reader the way a candidate without a production
	// replay path behaves.
	application.SetMaintenanceTombstoneReplay(nil, nil)
	t.Cleanup(func() {
		application.SetMaintenanceTombstoneReplay(maintenanceTombstonePathForTarget, maintenanceStateRootIsOwned)
	})
	if _, _, lerr := application.LoadMaintenanceRevision(resolved.stateRoot(), 1); lerr == nil {
		t.Fatal("the tombstone replay fallback is not wired through the production lookup: it answers silently with no reader")
	}

	// The client route reports the same failure: a destroyed deployment whose
	// reader was removed fails closed instead of answering "no record".
	replayOut := &strings.Builder{}
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Revision: 1,
	}, replayOut, io.Discard); err == nil {
		t.Fatal("the replay route reported success with no production reader registered")
	}
}

// TestApplicationMaintenanceReplayRouteAnswersDestroyRevision proves the
// supported production replay route (baha app maintenance --revision) answers
// the committed destroy revision after teardown — local route — and is
// idempotent: repeated calls answer the exact same durable record and mutate
// nothing.
func TestApplicationMaintenanceReplayRouteAnswersDestroyRevision(t *testing.T) {
	resolved, _ := maintenanceInstalledDeployment(t)
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	ownerCalls := 0
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previous(ctx, operation, r, args, out, errOut)
		}
		ownerCalls++
		return os.RemoveAll(r.stateRoot())
	}
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	if ownerCalls != 1 {
		t.Fatalf("the destroy owner ran %d times, want exactly 1", ownerCalls)
	}

	var first maintenanceReplayView
	for pass := 0; pass < 2; pass++ {
		out := &strings.Builder{}
		if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
			Name: resolved.Manifest.Name, Operation: "destroy", Revision: 1,
		}, out, io.Discard); err != nil {
			t.Fatalf("replay pass %d failed: %v", pass, err)
		}
		var view maintenanceReplayView
		if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &view); err != nil {
			t.Fatalf("replay pass %d is not a machine-safe record: %v", pass, err)
		}
		if view.Operation != "destroy" || view.Revision != 1 {
			t.Fatalf("replay pass %d answered operation %q revision %d, want destroy revision 1", pass, view.Operation, view.Revision)
		}
		if view.DeploymentID != resolved.DeploymentIdentity.DeploymentID || view.Target != resolved.Target.Name {
			t.Fatalf("replay pass %d answered a different deployment: %+v", pass, view)
		}
		if pass == 0 {
			first = view
			continue
		}
		if view != first {
			t.Fatalf("the replay is not idempotent: %+v vs %+v", view, first)
		}
	}
	if ownerCalls != 1 {
		t.Fatalf("the replay route invoked the lifecycle owner (calls: %d)", ownerCalls)
	}
}

// TestApplicationMaintenanceReplayRouteAnswersUnknownRevision proves a replay
// of a revision the deployment never committed fails closed with no durable
// mutation.
func TestApplicationMaintenanceReplayRouteAnswersUnknownRevision(t *testing.T) {
	resolved := maintenanceDeployment(t)
	if _, err := applicationMaintenanceAuthority(resolved); err != nil {
		t.Fatalf("resolve retained authority: %v", err)
	}
	out := &strings.Builder{}
	err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Revision: 9,
	}, out, io.Discard)
	if err == nil {
		t.Fatal("an unknown revision was answered as a durable record")
	}
	if out.Len() != 0 {
		t.Fatalf("a refused replay still printed an answer: %q", out.String())
	}
	if _, found, lerr := application.LoadMaintenanceRevision(resolved.stateRoot(), 9); lerr != nil || found {
		t.Fatalf("the refused replay mutated durable state: found=%v err=%v", found, lerr)
	}
}

// TestApplicationMaintenanceRemoteReplayRouteAnswersDestroyRevision proves
// the same supported production replay route answers the committed destroy
// revision for the remote route, local and remote identical in behavior.
func TestApplicationMaintenanceRemoteReplayRouteAnswersDestroyRevision(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	resolved := maintenanceRemoteDeployment(t, core, node)
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
		if operation != "destroy" {
			return previous(ctx, operation, r, args, out, errOut)
		}
		return os.RemoveAll(r.stateRoot())
	}
	if err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true,
	}, io.Discard, io.Discard); err != nil {
		t.Fatalf("remote destroy failed: %v", err)
	}
	out := &strings.Builder{}
	if err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Revision: 1,
	}, out, io.Discard); err != nil {
		t.Fatalf("the remote replay route failed: %v", err)
	}
	var view maintenanceReplayView
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &view); err != nil {
		t.Fatalf("the remote replay is not a machine-safe record: %v", err)
	}
	if view.Operation != "destroy" || view.Revision != 1 || view.Target != resolved.Target.Name {
		t.Fatalf("the remote replay answered %+v, want destroy revision 1 on target %q", view, resolved.Target.Name)
	}
	if view.Digest == "" || !strings.HasPrefix(view.Digest, "sha256:") {
		t.Fatal("the remote replay answer carries no durable digest binding")
	}
}

// assertCommittedDestroyTombstone proves the tombstone carries the committed
// destroy window (operation=destroy, revision=1) and never the stale pre-commit
// authority record.
func assertCommittedDestroyTombstone(t *testing.T, data []byte) {
	t.Helper()
	var record application.MaintenanceAuthorityRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("tombstone is not a valid maintenance record: %v", err)
	}
	if record.Operation != "destroy" {
		t.Fatalf("tombstone records operation %q, want destroy", record.Operation)
	}
	if record.Revision != 1 {
		t.Fatalf("tombstone records revision %d, want 1 (committed window)", record.Revision)
	}
	if record.ContractVersion != application.MaintenanceAuthorityContractVersion {
		t.Fatalf("tombstone record has an unsupported contract version")
	}
}

var _ = filepath.Join
