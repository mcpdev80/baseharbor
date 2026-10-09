package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// rollOwnedCoreHAPostgres is the only admitted mutation for an owned HA SQL
// delta. It verifies durable physical and isolated DCS recovery before
// staging the new Spilo image or replacing any Patroni member.
func rollOwnedCoreHAPostgres(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, delta coreupdate.Delta, installation, target, release, journalDir string) error {
	if rt == nil || !files.HA || files.Project == "" || files.Compose == "" ||
		installation == "" || target == "" || release == "" || journalDir == "" ||
		delta.Installed.Kind != coreupdate.SQL || delta.Installed.Scope != "shared" ||
		delta.Installed.Instance != "postgres-member-1" ||
		delta.Classification != coreupdate.BackupRequired {
		return errors.New("UNSUPPORTED: unowned or unadmitted Core HA PostgreSQL migration")
	}
	previous := coreupdate.BackingPin{
		Role: "core-ha-postgresql", Version: delta.Installed.Version,
		Image: delta.Installed.Image, Digest: delta.Installed.Digest,
	}
	desired := coreupdate.BackingPin{
		Role: "core-ha-postgresql", Version: delta.Desired.Version,
		Image: delta.Desired.Image, Digest: delta.Desired.Digest,
	}
	if err := verifyCoreHADCSSecurity(files); err != nil {
		return err
	}
	bridge, err := buildCoreEtcdRecoveryBridge(ctx, rt, files, installation, target, release, journalDir)
	if err != nil {
		return fmt.Errorf("bind authenticated HA PostgreSQL DCS: %w", err)
	}
	ops := &patroniCoreRollingOps{
		runtime: rt, files: files,
		journal: coreupdate.PatroniMemberJournal{
			Path: filepath.Join(journalDir, "patroni-members.json"), Release: release,
			Installation: installation, Scope: "shared", Desired: delta.Desired,
		},
		backup: coreupdate.StreamRecoveryPoint{
			Directory: filepath.Join(journalDir, "patroni-recovery"), Name: "core-spilo-basebackup",
		},
	}
	compose := coreupdate.HAPostgresComposeCheckpoint{
		Path: files.Compose, Directory: filepath.Join(journalDir, "ha-spilo-compose"),
		Previous: previous, Desired: desired,
	}
	checkpoint := coreupdate.DCSCheckpoint{Path: filepath.Join(journalDir, "dcs-recovery.json")}
	if err := coreupdate.StageAndRollPatroniCluster(ctx, ops, bridge, checkpoint, compose,
		installation, target, bridge.Store.Identity.Cluster, release, 0); err != nil {
		return fmt.Errorf("verified owned Patroni rolling migration failed; recovery evidence retained: %w", err)
	}
	members, err := ops.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect rolled Patroni members: %w", err)
	}
	if _, _, err := coreupdate.VerifyPatroniQuorum(ctx, members, 0); err != nil {
		return fmt.Errorf("final Patroni quorum after rolling upgrade: %w", err)
	}
	for _, name := range files.PostgresMembers() {
		if err := ops.VerifyMemberImage(ctx, name); err != nil {
			return fmt.Errorf("final Patroni member image verification: %w", err)
		}
	}
	return nil
}
