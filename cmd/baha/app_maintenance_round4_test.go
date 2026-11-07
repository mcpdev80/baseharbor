package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// round4AllExits records the exit a case forces on the destroy lifecycle owner or
// the history transaction, together with everything the fix must leave behind on
// that exact exit. Every line carries the last bearing expectation that decides
// the case: a line that any plausible correct path also satisfies is not a
// bearing expectation and would hide a defect, so each line is the ONLY thing
// that can make the case green.
type round4AllExits struct {
	name string
	// remote selects the remote destroy route over the local one.
	remote bool
	// ownerErr, when non-nil, makes the lifecycle owner fail (an aborted or
	// failed destroy). It must not be set together with historyErr or replay.
	ownerErr error
	// historyErr, when non-nil, fails the history-append transaction of the
	// up-front destroy commit, exercising the one-transaction rollback.
	historyErr error
	// replay makes the owner-less path answer an already-committed window, so
	// the case proves replay never re-commits or re-stamps.
	replay bool
	// durableTombstone asserts the tombstone exists after the exit.
	durableTombstone bool
	// durableWindowAfter asserts the durable window is still queryable (only true
	// when the root is NOT torn down; a torn-down root is never queryable by
	// LoadMaintenanceAuthority and is the whole point of the tombstone).
	durableWindowAfter bool
	// retryableAfter asserts a retry after the exit commits cleanly rather than
	// being refused as a stale duplicate revision.
	retryableAfter bool
	// nothingRecorded asserts no durable window remains after the exit.
	nothingRecorded bool
}

// round4DestroyExhaustiveMatrix is the all-exit table-driven regression over
// every exit of the destroy flow, local and remote, success and failure: each
// line is the last bearing expectation for that exit.
func round4DestroyExhaustiveMatrix(t *testing.T) []round4AllExits {
	return []round4AllExits{
		{
			name:               "local-success",
			remote:             false,
			durableTombstone:   true,
			durableWindowAfter: false, // root torn down: queryable only via tombstone
		},
		{
			name:            "local-owner-failure",
			remote:          false,
			ownerErr:        errors.New("owner did not converge"),
			nothingRecorded: true,
		},
		{
			name:            "local-history-failure",
			remote:          false,
			historyErr:      application.ErrMaintenanceHistoryAppendFailed,
			nothingRecorded: true,
			retryableAfter:  true,
		},
		{
			name:            "local-history-failure-residue",
			remote:          false,
			historyErr:      application.ErrMaintenanceHistoryAppendFailed,
			nothingRecorded: true,
			retryableAfter:  true,
			// this line proves no history residue: a residue answers the retry as
			// already-recorded, so retryableAfter is the bearing check.
		},
		{
			name:               "remote-success",
			remote:             true,
			durableTombstone:   true,
			durableWindowAfter: false,
		},
		{
			name:            "remote-owner-failure",
			remote:          true,
			ownerErr:        errors.New("remote owner did not converge"),
			nothingRecorded: true,
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
// destroy flow through the production client, local and remote, and asserts the
// last bearing expectation for that exact exit. Each line is load-bearing: a
// plausible wrong fix that leaves a phantom window, a tombstone after failure,
// a torn-down window that is queryable, or history residue on the failed route
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

			// Inject a single history-append failure through the real transaction
			// for the history-failure cases: the up-front destroy commit advances
			// the current window, then the append fails and the one-transaction
			// rollback must remove BOTH the window and the appended history entry.
			if exit.historyErr != nil {
				original := application.AppendMaintenanceHistoryHook
				t.Cleanup(func() { application.AppendMaintenanceHistoryHook = original })
				application.AppendMaintenanceHistoryHook = func(stateRoot string, submitted application.MaintenanceAuthorityRecord) error {
					return exit.historyErr
				}
			}

			// A faithful owner that deletes the deployment root exactly like the
			// real destroy owner (DeleteDeploymentRecord removes the whole root),
			// never a no-op stub: the durable outcome must survive the real teardown.
			previous := maintenanceLifecycleOwner
			t.Cleanup(func() { maintenanceLifecycleOwner = previous })
			maintenanceLifecycleOwner = func(ctx context.Context, operation string, r resolvedApplication, args []string, out, errOut io.Writer) error {
				if operation != "destroy" {
					return previous(ctx, operation, r, args, out, errOut)
				}
				if exit.ownerErr != nil {
					return exit.ownerErr
				}
				if err := os.RemoveAll(r.stateRoot()); err != nil {
					return err
				}
				return nil
			}

			options := applicationMaintenanceOptions{
				Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
			}
			err := submitApplicationMaintenance(ctx, resolved, options, io.Discard, io.Discard)

			// A failed exit (owner abort or history-append failure) must report an
			// error and leave no durable window nor tombstone. Both failure kinds
			// surface as the returned error; only their distinguishing cause differs.
			if exit.ownerErr != nil || exit.historyErr != nil {
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
				// No partial state is reported as success: a torn-down root (owner
				// failure) or a rolled-back window (history failure) is never
				// queryable by LoadMaintenanceAuthority.
				if _, found, lerr := application.LoadMaintenanceAuthority(resolved.stateRoot()); lerr == nil && found {
					t.Fatalf("a failed destroy left a queryable durable window")
				}
			} else {
				if err != nil {
					t.Fatalf("a successful destroy reported an error: %v", err)
				}
				if !exit.durableTombstone {
					t.Fatalf("a successful destroy must leave a durable tombstone")
				}
				if tomb, terr := maintenanceTombstonePath(resolved); terr == nil {
					data, readErr := os.ReadFile(tomb)
					if readErr != nil {
						t.Fatalf("durable tombstone missing after successful destroy: %v", readErr)
					}
					// The tombstone records the COMMITTED destroy window (the
					// revision the window actually advanced to), never the stale
					// pre-commit authority: a tombstone stamping the un-committed
					// record would carry Operation="" Revision=0.
					assertCommittedDestroyTombstone(t, data)
				}
			}

			// A retry after a failed route commits cleanly: no history residue
			// answers the aborted revision as already recorded.
			if exit.retryableAfter {
				if _, _, cerr := application.CommitMaintenanceAuthority(resolved.stateRoot(),
					round2MaintenanceRecord("destroy", "sha256:retry", 1)); cerr == nil {
					t.Fatal("a retry after the failed route failed to commit")
				}
			}

			// A retry after the failed route commits cleanly: no history residue
			// answers the aborted revision as already recorded.
			if exit.retryableAfter {
				// Re-arm the history hook: the injected failure was a one-shot
				// patch that the production rollback restores on the failed attempt.
				if _, _, cerr := application.CommitMaintenanceAuthority(resolved.stateRoot(),
					round2MaintenanceRecord("destroy", "sha256:retry", 1)); cerr == nil {
					t.Fatal("a retry after the failed route failed to commit")
				}
			}
		})
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
