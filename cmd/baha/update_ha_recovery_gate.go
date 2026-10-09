package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func validateHAProviderPlan(plan coreupdate.Plan) error {
	for _, delta := range plan.Deltas {
		if delta.Classification == coreupdate.NoChange {
			continue
		}
		if delta.Installed.Kind != coreupdate.Secrets && delta.Installed.Kind != coreupdate.Identity {
			return fmt.Errorf("UNSUPPORTED: HA backing provider %s must remain unchanged until its rolling member migration is explicitly requested", delta.Installed.Instance)
		}
		switch delta.Classification {
		case coreupdate.BackupRequired, coreupdate.MigrationRequired:
		default:
			return fmt.Errorf("UNSUPPORTED: HA provider %s lacks an admitted backup-backed migration: %s", delta.Installed.Instance, delta.Reason)
		}
	}
	return nil
}

func prepareCoreHARecoveryEvidence(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, state coreinstallation.State, targetName, release, journalDir string) error {
	if rt == nil || !files.HA {
		return errors.New("HA recovery evidence requires an owned HA Core runtime")
	}
	before, err := inspectPatroniMembers(ctx, rt, files)
	if err != nil {
		return fmt.Errorf("inspect HA Patroni members before recovery capture: %w", err)
	}
	leader, _, err := coreupdate.VerifyPatroniQuorum(ctx, before, 0)
	if err != nil {
		return fmt.Errorf("Core HA Patroni quorum before recovery capture: %w", err)
	}
	if err := captureOwnedPatroniBackup(ctx, rt, files, filepath.Join(journalDir, "patroni-recovery")); err != nil {
		return fmt.Errorf("capture verified Patroni physical recovery point: %w", err)
	}
	bridge, err := buildCoreEtcdRecoveryBridge(ctx, rt, files, state.ID, targetName, release, journalDir)
	if err != nil {
		return fmt.Errorf("bind etcd DCS recovery adapter: %w", err)
	}
	if _, err := coreupdate.CaptureAndVerifyDCS(ctx, bridge, state.ID, targetName, bridge.Store.Identity.Cluster, release); err != nil {
		return fmt.Errorf("capture and verify etcd DCS recovery evidence: %w", err)
	}
	after, err := inspectPatroniMembers(ctx, rt, files)
	if err != nil {
		return fmt.Errorf("inspect HA Patroni members after DCS recovery proof: %w", err)
	}
	afterLeader, _, err := coreupdate.VerifyPatroniQuorum(ctx, after, 0)
	if err != nil {
		return fmt.Errorf("Core HA Patroni quorum after DCS recovery proof: %w", err)
	}
	if afterLeader != leader {
		return errors.New("Patroni leader changed while establishing recovery evidence; replan required")
	}
	return nil
}
