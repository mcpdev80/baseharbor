package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func verifyCoreHADCSSecurity(files bhruntime.Files) error {
	if !files.HA || files.Compose == "" {
		return errors.New("HA DCS security verification requires managed HA runtime files")
	}
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		return err
	}
	text := string(data)
	for _, forbidden := range []string{"--listen-client-urls=http://", "--listen-peer-urls=http://", "=http://postgres-etcd-"} {
		if strings.Contains(text, forbidden) {
			return errors.New("UNSUPPORTED: existing plaintext etcd HA topology requires explicit fenced mTLS migration")
		}
	}
	for _, required := range []string{
		"--client-cert-auth=true",
		"--peer-client-cert-auth=true",
		"--trusted-ca-file=/run/baseharbor/etcd/ca.pem",
		"--peer-trusted-ca-file=/run/baseharbor/etcd/ca.pem",
		"postgres-etcd-recovery:",
		"PATRONI_ETCD3_PROTOCOL: https",
		"ETCD3_PROTOCOL: https",
		"ETCD3_CACERT: /run/baseharbor/tls-runtime/etcd-ca.pem",
		"ETCD3_CERT: /run/baseharbor/tls-runtime/etcd-client.pem",
		"ETCD3_KEY: /run/baseharbor/tls-runtime/etcd-client-key.pem",
	} {
		if !strings.Contains(text, required) {
			return fmt.Errorf("UNSUPPORTED: managed HA DCS is missing required security contract %q", required)
		}
	}
	pkiDir := filepath.Join(filepath.Dir(files.Compose), "providers", "postgresql", "runtime", "etcd-pki")
	_, err = (etcdbackup.TLSFiles{
		CA: filepath.Join(pkiDir, "ca.pem"), Cert: filepath.Join(pkiDir, "client.pem"), Key: filepath.Join(pkiDir, "client-key.pem"),
	}).Config()
	if err != nil {
		return fmt.Errorf("managed HA DCS recovery mTLS identity invalid: %w", err)
	}
	return nil
}

func validateHAProviderPlan(plan coreupdate.Plan) error {
	for _, delta := range plan.Deltas {
		if delta.Classification == coreupdate.NoChange {
			continue
		}
		if delta.Installed.Kind == coreupdate.SQL && delta.Installed.Scope == "shared" &&
			delta.Installed.Instance == "postgres-member-1" && delta.Classification == coreupdate.BackupRequired {
			continue // separately executed by the verified HA Patroni rolling coordinator
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
	if err := verifyCoreHADCSSecurity(files); err != nil {
		return err
	}
	leader, err := awaitOwnedPatroniQuorum(ctx, rt, files, "")
	if err != nil {
		return fmt.Errorf("Core HA Patroni quorum before recovery capture: %w", err)
	}
	if err := captureOwnedPatroniBackupFromLeader(ctx, rt, files, filepath.Join(journalDir, "patroni-recovery"), leader); err != nil {
		return fmt.Errorf("capture verified Patroni physical recovery point: %w", err)
	}
	if err := prepareOwnedPatroniPhysicalRestore(ctx, journalDir); err != nil {
		return fmt.Errorf("materialize and verify isolated PostgreSQL physical recovery data: %w", err)
	}
	bridge, err := buildCoreEtcdRecoveryBridge(ctx, rt, files, state.ID, targetName, release, journalDir)
	if err != nil {
		return fmt.Errorf("bind etcd DCS recovery adapter: %w", err)
	}
	ev, err := (coreupdate.DCSCheckpoint{Path: filepath.Join(journalDir, "dcs-recovery.json")}).Acquire(ctx, bridge, state.ID, targetName, bridge.Store.Identity.Cluster, release)
	if err != nil {
		return fmt.Errorf("capture and verify etcd DCS recovery evidence: %w", err)
	}
	if err := captureCoreHARecoverySource(ctx, rt, files, journalDir, leader, ev); err != nil {
		return err
	}
	_, err = awaitOwnedPatroniQuorum(ctx, rt, files, leader)
	if err != nil {
		return fmt.Errorf("Core HA Patroni quorum after DCS recovery proof: %w", err)
	}
	return nil
}
