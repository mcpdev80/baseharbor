package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type coreNativeRuntimeOps struct {
	runtime                                                     bhruntime.RuntimeProvider
	core                                                        bhruntime.Files
	identity                                                    identityprovider.KeycloakFiles
	dataDir, target, installation, issuer, release, receiptPath string
}

func (o *coreNativeRuntimeOps) files(d coreupdate.Delta) (string, string, string) {
	if d.Installed.Kind == coreupdate.Identity || d.Installed.Scope == "backing" {
		return o.identity.Project, o.identity.Compose, o.identity.Env
	}
	return o.core.Project, o.core.Compose, o.core.Env
}
func (o *coreNativeRuntimeOps) Preflight(ctx context.Context, plan coreupdate.Plan) error {
	if len(plan.Deltas) != 4 {
		return fmt.Errorf("Core runtime update requires four owned SQL/Secrets/Identity/Keycloak-backing realizations")
	}
	for _, files := range []struct{ project, compose, env string }{{o.core.Project, o.core.Compose, o.core.Env}, {o.identity.Project, o.identity.Compose, o.identity.Env}} {
		if err := o.runtime.ConfigProject(ctx, files.project, files.compose, files.env); err != nil {
			return fmt.Errorf("Core managed project %s preflight: %w", files.project, err)
		}
	}
	return nil
}
func (o *coreNativeRuntimeOps) Quiesce(ctx context.Context, d coreupdate.Delta) error {
	project, compose, env := o.files(d)
	if err := o.runtime.StopProject(ctx, project, compose, env); err != nil {
		return fmt.Errorf("stop owned Core provider project %s: %w", project, err)
	}
	return o.verifyQuiesced(ctx, project, "")
}
func (o *coreNativeRuntimeOps) verifyQuiesced(ctx context.Context, project, volume string) error {
	containers, err := o.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return err
	}
	for _, c := range containers {
		if c.Project == project && c.Running {
			return fmt.Errorf("Core volume %s project %s still has active service %s", volume, project, c.Service)
		}
	}
	return nil
}
func (o *coreNativeRuntimeOps) ReconcilePinned(ctx context.Context, d coreupdate.Delta) error {
	project, compose, env := o.files(d)
	if err := o.runtime.UpProject(ctx, project, compose, env); err != nil {
		return err
	}
	return o.verifyImage(ctx, d, true)
}
func (o *coreNativeRuntimeOps) ReconcileOriginal(ctx context.Context, d coreupdate.Delta) error {
	project, compose, env := o.files(d)
	if err := o.runtime.UpProject(ctx, project, compose, env); err != nil {
		return err
	}
	return o.verifyImage(ctx, d, false)
}
func (o *coreNativeRuntimeOps) verifyImage(ctx context.Context, d coreupdate.Delta, pinned bool) error {
	project, _, _ := o.files(d)
	id, err := o.runtime.ProjectServiceImageIdentity(ctx, project, d.Installed.Instance)
	if err != nil {
		return err
	}
	expected := d.Installed.Digest
	if pinned {
		expected = d.Desired.Digest
	}
	digest := id.Digest
	if i := strings.Index(digest, "@sha256:"); i >= 0 {
		digest = digest[i+1:]
	}
	if digest != expected {
		return fmt.Errorf("Core provider %s running digest %s differs from expected pinned image", d.Installed.Instance, digest)
	}
	return nil
}
func (o *coreNativeRuntimeOps) VerifySemantics(ctx context.Context, d coreupdate.Delta) error {
	if _, ok := health.Format(health.RuntimeChecksForFiles(o.core)); !ok {
		return errors.New("Core PostgreSQL/OpenBao health check failed")
	}
	if err := platformopenbao.CheckManager(ctx, o.runtime, o.core); err != nil {
		return fmt.Errorf("OpenBao semantics: %w", err)
	}
	if err := identityprovider.VerifyCoreIdentity(ctx, o.dataDir, o.target, o.installation, o.issuer); err != nil {
		return fmt.Errorf("Keycloak OIDC semantics: %w", err)
	}
	return nil
}
func (o *coreNativeRuntimeOps) Record(_ context.Context, d coreupdate.Delta, state string) error {
	journal, err := coreupdate.LoadJournal(o.receiptPath, o.release)
	if err != nil {
		return err
	}
	return journal.Record(o.receiptPath, d, state)
}

func nativeRecoveryService(d coreupdate.Delta) string {
	switch d.Installed.Kind {
	case coreupdate.Identity:
		return "keycloak-db"
	case coreupdate.Secrets:
		return "postgres-member-1"
	default:
		return d.Installed.Instance
	}
}

func reconcileNativeCoreProviders(ctx context.Context, release string) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	stateRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	state, err := coreinstallation.Load(stateRoot)
	if err != nil {
		return err
	}
	if !state.Ready || state.ID == "" || state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return errors.New("refusing provider upgrade for unready or mismatched owned Core")
	}
	deployed, err := deployment.ListDeployments(target.Name)
	if err != nil {
		return err
	}
	if len(deployed) > 0 {
		return errors.New("Core provider upgrade requires application-isolated migration plan for registered deployments")
	}
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	plan, err := inspectCoreRuntimePlan(ctx, release, state, runtime)
	if err != nil {
		return err
	}
	if len(plan.Deltas) != 4 {
		return errors.New("incomplete managed Core/backing provider realization")
	}
	changed := false
	for _, d := range plan.Deltas {
		if d.Classification == coreupdate.Unsupported {
			return fmt.Errorf("unsupported provider %s upgrade: %s", d.Installed.Instance, d.Reason)
		}
		if d.Classification != coreupdate.NoChange {
			changed = true
		}
	}
	if !changed {
		return verifyCoreBinaryOnly(ctx, release)
	}
	if state.Spec.HA {
		return errors.New("HA Core provider change UNSUPPORTED: verified rolling Spilo/Patroni backup, replica checks and recovery are required; no mutation attempted")
	}
	coreFiles, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return err
	}
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return err
	}
	identityFiles, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target.Name)
	if err != nil {
		return err
	}
	journalDir := filepath.Join(stateRoot, "core-updates", safeVersionPathPart(release))
	if err := os.MkdirAll(journalDir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(journalDir, 0700); err != nil {
		return err
	}
	ops := &coreNativeRuntimeOps{runtime: runtime, core: coreFiles, identity: identityFiles, dataDir: dataDir, target: target.Name,
		installation: state.ID, issuer: state.IdentityIssuer, release: release, receiptPath: filepath.Join(journalDir, "receipts.json")}
	assets := map[string]coreupdate.NativeProviderAssets{}
	for _, d := range plan.Deltas {
		if d.Classification == coreupdate.NoChange {
			continue
		}
		project, compose, _ := ops.files(d)
		dataService := nativeRecoveryService(d)
		volume, volumeErr := coreupdate.ResolveOwnedServiceVolume(compose, dataService, project)
		if volumeErr != nil {
			return fmt.Errorf("Core %s requires a verifiable persistent data volume before migration: %w", d.Installed.Instance, volumeErr)
		}
		assets[coreupdate.JournalKey(d)] = coreupdate.NativeProviderAssets{
			Recovery: coreupdate.VolumeRecovery{Runtime: runtime, Directory: filepath.Join(journalDir, "backups"), Project: project, Volume: volume, VerifyQuiesced: ops.verifyQuiesced},
			Compose:  coreupdate.ComposeCheckpoint{Path: compose, Directory: filepath.Join(journalDir, "compose-backups")},
		}
	}
	if err := coreupdate.RunNativeProviderUpdates(ctx, plan, filepath.Join(journalDir, "journal.json"), ops, assets); err != nil {
		return err
	}
	return verifyCoreBinaryOnly(ctx, release)
}

func preflightNativeCoreUpgrade(ctx context.Context, release string) error {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		return err
	}
	state, err := coreinstallation.Load(root)
	if err != nil {
		return err
	}
	if !state.Ready || state.ID == "" || state.Spec.Target != target.Name || state.Spec.Runtime != target.RuntimeProvider {
		return errors.New("Core installation not owned and ready for provider upgrade")
	}
	records, err := deployment.ListDeployments(target.Name)
	if err != nil {
		return err
	}
	if len(records) > 0 {
		return errors.New("Core provider upgrade blocked until registered application-scoped provider migrations can be verified")
	}
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	plan, err := inspectCoreRuntimePlan(ctx, release, state, runtime)
	if err != nil {
		return err
	}
	if len(plan.Deltas) != 4 {
		return errors.New("incomplete SQL/Secrets/Identity/backing inventory")
	}
	for _, d := range plan.Deltas {
		if state.Spec.HA && d.Classification != coreupdate.NoChange {
			return fmt.Errorf("HA Core provider %s requires a verified rolling migration (UNSUPPORTED): %s", d.Installed.Instance, d.Reason)
		}
		switch d.Classification {
		case coreupdate.NoChange, coreupdate.BackupRequired, coreupdate.MigrationRequired:
		default:
			return fmt.Errorf("Core provider %s requires unsupported migration: %s", d.Installed.Instance, d.Reason)
		}
	}
	return nil
}
