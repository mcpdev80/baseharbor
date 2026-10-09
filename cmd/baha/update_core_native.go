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
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/providerbinding"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	keycloakadapter "github.com/mcpdev80/baseharbor/internal/providerupgrade/keycloak"
	baoAdapter "github.com/mcpdev80/baseharbor/internal/providerupgrade/openbao"
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

// verifyOpenBaoBackingSQL proves that the protected OpenBao application
// credentials can authenticate against their actual, dedicated SQL database.
// Passwords are passed via stdin by probeControlPlanePostgresCredential.
func (o *coreNativeRuntimeOps) verifyOpenBaoBackingSQL(ctx context.Context) error {
	credentials, err := bhruntime.LoadControlPlaneCredentials(o.core)
	if err != nil {
		return fmt.Errorf("managed OpenBao SQL credentials unavailable: %w", err)
	}
	if err := probeControlPlanePostgresCredential(ctx, o.runtime, o.core, credentials.OpenBaoDBUser, credentials.OpenBaoDBPassword, "openbao"); err != nil {
		return fmt.Errorf("OpenBao backing SQL authentication/readiness failed: %w", err)
	}
	return nil
}

// verifyKeycloakBackingSQL authenticates with the dedicated Keycloak
// application role over verified TLS. It does not use the superuser or log
// credentials, and runs only in the owned single-Core topology.
func (o *coreNativeRuntimeOps) verifyKeycloakBackingSQL(ctx context.Context) error {
	files := bhruntime.Files{Project: o.identity.Project, Compose: o.identity.Compose, Env: o.identity.Env}
	values, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		return fmt.Errorf("protected Keycloak SQL credentials unavailable: %w", err)
	}
	user, password, database := values["BASEHARBOR_KEYCLOAK_DB_USER"], values["BASEHARBOR_KEYCLOAK_DB_PASSWORD"], values["BASEHARBOR_KEYCLOAK_DB_NAME"]
	if user == "" || password == "" || database == "" {
		return errors.New("Keycloak SQL owner credentials are incomplete")
	}
	const script = "IFS= read -r PGPASSWORD || exit 1\nexport PGPASSWORD PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/db-tls/ca.pem PGCONNECT_TIMEOUT=5\nexec psql -h keycloak-db -p 5432 -U \"$1\" -d \"$2\" -Atqc \"SELECT CASE WHEN to_regclass('public.realm') IS NOT NULL AND to_regclass('public.client') IS NOT NULL THEN '1' ELSE 'missing_keycloak_schema' END\" -v ON_ERROR_STOP=1"
	output, err := o.runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(password+"\n"), "keycloak-db", "sh", "-ec", script, "--", user, database)
	if err != nil {
		return fmt.Errorf("Keycloak managed SQL user cannot authenticate over TLS: %w", err)
	}
	if strings.TrimSpace(output) != "1" {
		return errors.New("Keycloak SQL realm/client schema not verified")
	}
	return nil
}

func (o *coreNativeRuntimeOps) inspectNativeKeycloakMember(ctx context.Context, service string) (keycloakadapter.State, error) {
	members, err := o.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return keycloakadapter.State{}, err
	}
	found := 0
	for _, member := range members {
		if member.Project != o.identity.Project || member.Service != service {
			continue
		}
		found++
		if !member.Running || strings.EqualFold(member.Health, "unhealthy") {
			return keycloakadapter.State{}, errors.New("Keycloak managed member not healthy")
		}
	}
	if found != 1 {
		return keycloakadapter.State{}, errors.New("Keycloak single topology requires exactly one owned member")
	}
	image, err := o.runtime.ProjectServiceImageIdentity(ctx, o.identity.Project, service)
	if err != nil {
		return keycloakadapter.State{}, err
	}
	ref := strings.Split(image.Reference, "@")[0]
	index := strings.LastIndex(ref, ":")
	if index < 0 || index == len(ref)-1 {
		return keycloakadapter.State{}, errors.New("Keycloak runtime image version is not observable")
	}
	version := ref[index+1:]
	return keycloakadapter.State{Version: version, Owner: "baseharbor", Topology: keycloakadapter.TopologySingle, Healthy: true,
		DatabaseType: "postgresql", Members: []keycloakadapter.Member{{Name: service, Version: version, Ready: true}}}, nil
}

// admitNativeProviderTransition delegates conservative single-Core patch upgrades
// to the actual OpenBao/Keycloak adapter Preflight contracts while the original
// installation is still running. Snapshot and mutation remain journal-owned.
func (o *coreNativeRuntimeOps) admitNativeProviderTransition(ctx context.Context, d coreupdate.Delta) error {
	if d.Classification == coreupdate.NoChange {
		return nil
	}
	request := providerupgrade.Request{CurrentVersion: d.Installed.Version, TargetVersion: d.Desired.Version,
		TargetImage: d.Desired.Image, TargetDigest: d.Desired.Digest}
	patchOnly := func(_ context.Context, from, to string) error {
		old, err := providerupgrade.ParseVersion(from)
		if err != nil {
			return err
		}
		next, err := providerupgrade.ParseVersion(to)
		if err != nil {
			return err
		}
		if !old.SameMinor(next) || old.Compare(next) >= 0 {
			return errors.New("UNSUPPORTED: native provider upgrade requires a strictly newer patch within the same major/minor")
		}
		return nil
	}
	switch d.Installed.Kind {
	case coreupdate.Secrets:
		adapter := baoAdapter.New(&baoAdapter.NativeOps{Executor: o.runtime, Files: o.core, Owner: "baseharbor",
			Hooks: baoAdapter.RuntimeHooks{UpgradePath: patchOnly}})
		assessment, err := adapter.Preflight(ctx, request)
		if err != nil {
			return err
		}
		if assessment.Classification != providerupgrade.ClassificationSupported || !assessment.BackupRequired {
			return errors.New("UNSUPPORTED: OpenBao mutation requires verified provider-adapter backup admission")
		}
		return nil
	case coreupdate.Identity:
		adapter := keycloakadapter.New(&keycloakadapter.NativeOps{
			DataDir: o.dataDir, Namespace: o.target, InstallationID: o.installation, ExpectedIssuer: o.issuer,
			Hooks: keycloakadapter.CoreHooks{
				Inspect: func(ctx context.Context) (keycloakadapter.State, error) {
					return o.inspectNativeKeycloakMember(ctx, d.Installed.Instance)
				},
				Compatibility: patchOnly,
			},
		})
		assessment, err := adapter.Preflight(ctx, request)
		if err != nil {
			return err
		}
		if assessment.Classification != providerupgrade.ClassificationSupported || !assessment.BackupRequired {
			return errors.New("UNSUPPORTED: Keycloak mutation requires verified provider-adapter backup admission")
		}
		return nil
	}
	return nil
}

func (o *coreNativeRuntimeOps) Preflight(ctx context.Context, plan coreupdate.Plan) error {
	if len(plan.Deltas) != 4 {
		return fmt.Errorf("Core runtime update requires four owned SQL/Secrets/Identity/Keycloak-backing realizations")
	}
	for _, d := range plan.Deltas {
		if d.Installed.Kind == coreupdate.Secrets || d.Installed.Kind == coreupdate.Identity {
			if err := o.admitNativeProviderTransition(ctx, d); err != nil {
				return fmt.Errorf("%s provider-adapter upgrade admission: %w", d.Installed.Kind, err)
			}
		}
	}
	for _, files := range []struct{ project, compose, env string }{{o.core.Project, o.core.Compose, o.core.Env}, {o.identity.Project, o.identity.Compose, o.identity.Env}} {
		if err := o.runtime.ConfigProject(ctx, files.project, files.compose, files.env); err != nil {
			return fmt.Errorf("Core managed project %s preflight: %w", files.project, err)
		}
	}
	if err := o.verifyOpenBaoBackingSQL(ctx); err != nil {
		return err
	}
	if err := platformopenbao.VerifyUpgradeManagerPolicyAndAppRole(ctx, o.runtime, o.core); err != nil {
		return fmt.Errorf("OpenBao AppRole/policies/KV preflight: %w", err)
	}
	if err := o.verifyKeycloakBackingSQL(ctx); err != nil {
		return err
	}
	if err := identityprovider.VerifyCoreOperatorTokenFlow(ctx, o.dataDir, o.target, o.installation, o.issuer); err != nil {
		return fmt.Errorf("Keycloak SQL/realm/OIDC/token preflight: %w", err)
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

// verifyBoundProviderSemantics delegates post-upgrade validation to the
// integrated Session-1C adapters after the native runner has confirmed its
// immutable image. During rollback the original digest remains valid and the
// existing Core semantic checks are still mandatory.
func (o *coreNativeRuntimeOps) verifyBoundProviderSemantics(ctx context.Context, d coreupdate.Delta) error {
	var project, service string
	switch d.Installed.Kind {
	case coreupdate.Secrets:
		project, service = o.core.Project, d.Installed.Instance
	case coreupdate.Identity:
		project, service = o.identity.Project, d.Installed.Instance
	default:
		return nil
	}
	identity, err := o.runtime.ProjectServiceImageIdentity(ctx, project, service)
	if err != nil {
		return err
	}
	digest := identity.Digest
	if index := strings.Index(digest, "@sha256:"); index >= 0 {
		digest = digest[index+1:]
	}
	if digest == d.Installed.Digest {
		return nil
	}
	if digest != d.Desired.Digest {
		return errors.New("provider semantics cannot verify an unrecognized runtime image digest")
	}
	request := providerupgrade.Request{
		CurrentVersion: d.Installed.Version, TargetVersion: d.Desired.Version,
		TargetImage: d.Desired.Image, TargetDigest: d.Desired.Digest,
	}
	switch d.Installed.Kind {
	case coreupdate.Secrets:
		ops := &baoAdapter.NativeOps{Executor: o.runtime, Files: o.core, Owner: "baseharbor",
			Hooks: baoAdapter.RuntimeHooks{
				VerifyAuth: func(ctx context.Context) error {
					return platformopenbao.VerifyUpgradeManagerPolicyAndAppRole(ctx, o.runtime, o.core)
				},
				VerifyApps: func(ctx context.Context) error {
					return o.verifyOwnedApplicationSecretScopes(ctx)
				},
			},
		}
		return baoAdapter.New(ops).Verify(ctx, request)
	case coreupdate.Identity:
		ops := &keycloakadapter.NativeOps{
			DataDir: o.dataDir, Namespace: o.target, InstallationID: o.installation, ExpectedIssuer: o.issuer,
			Hooks: keycloakadapter.CoreHooks{
				Inspect: func(ctx context.Context) (keycloakadapter.State, error) {
					return o.inspectNativeKeycloakMember(ctx, service)
				},
				VerifySQL: o.verifyKeycloakBackingSQL,
				VerifyRealms: func(ctx context.Context) error {
					return identityprovider.VerifyCoreIdentity(ctx, o.dataDir, o.target, o.installation, o.issuer)
				},
				VerifyTokens: func(ctx context.Context) error {
					return identityprovider.VerifyCoreOperatorTokenFlow(ctx, o.dataDir, o.target, o.installation, o.issuer)
				},
			},
		}
		return keycloakadapter.New(ops).Verify(ctx, request)
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
	if err := o.verifyOpenBaoBackingSQL(ctx); err != nil {
		return err
	}
	if err := platformopenbao.VerifyUpgradeManagerPolicyAndAppRole(ctx, o.runtime, o.core); err != nil {
		return fmt.Errorf("OpenBao AppRole, policies and secret access: %w", err)
	}
	if err := o.verifyKeycloakBackingSQL(ctx); err != nil {
		return err
	}
	if err := identityprovider.VerifyCoreOperatorTokenFlow(ctx, o.dataDir, o.target, o.installation, o.issuer); err != nil {
		return fmt.Errorf("Keycloak realm, OIDC and operator token semantics: %w", err)
	}
	if err := o.verifyBoundProviderSemantics(ctx, d); err != nil {
		return fmt.Errorf("native provider adapter semantic verification: %w", err)
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
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	plan, err := inspectCoreRuntimePlan(ctx, release, state, runtime)
	if err != nil {
		return err
	}
	plan, err = corePlanOnly(plan)
	if err != nil {
		return err
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
	openBaoMembers := coreFiles.OpenBaoMembers()
	if len(openBaoMembers) != 1 {
		return errors.New("single-Core OpenBao upgrade requires exactly one owned member")
	}
	// Require live ownership and immutable image identity for both provider
	// adapters before any native update journal, backup or mutation is touched.
	binding := &providerbinding.RuntimeBinding{
		Reader: runtime, Engine: target.RuntimeProvider,
		Sources: map[providerupgrade.Provider]providerbinding.ManagedSource{
			providerupgrade.ProviderOpenBao:  {Project: coreFiles.Project, Service: openBaoMembers[0]},
			providerupgrade.ProviderKeycloak: {Project: identityFiles.Project, Service: "keycloak-1"},
		},
	}
	for _, kind := range []providerupgrade.Provider{providerupgrade.ProviderOpenBao, providerupgrade.ProviderKeycloak} {
		if _, err := binding.InspectManaged(ctx, kind); err != nil {
			return fmt.Errorf("managed %s runtime ownership preflight: %w", kind, err)
		}
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
	bound, err := ops.buildBoundProviderTransaction(ctx, plan, journalDir, target.RuntimeProvider)
	if err != nil {
		return fmt.Errorf("bind provider adapter transaction: %w", err)
	}
	assets := map[string]coreupdate.NativeProviderAssets{}
	for _, d := range plan.Deltas {
		if d.Classification == coreupdate.NoChange || d.Installed.Kind == coreupdate.Secrets || d.Installed.Kind == coreupdate.Identity {
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
	if err := coreupdate.RunMixedProviderUpdates(ctx, plan, filepath.Join(journalDir, "journal.json"), ops, assets, bound.Hooks(), func(d coreupdate.Delta) bool {
		return d.Installed.Kind == coreupdate.Secrets || d.Installed.Kind == coreupdate.Identity
	}); err != nil {
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
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	plan, err := inspectCoreRuntimePlan(ctx, release, state, runtime)
	if err != nil {
		return err
	}
	plan, err = corePlanOnly(plan)
	if err != nil {
		return err
	}
	if state.Spec.HA {
		files, filesErr := existingTargetRuntimeFiles(ctx)
		if filesErr != nil {
			return fmt.Errorf("inspect HA Core runtime files: %w", filesErr)
		}
		members, inspectErr := inspectPatroniMembers(ctx, runtime, files)
		if inspectErr != nil {
			return fmt.Errorf("inspect HA Patroni members: %w", inspectErr)
		}
		if _, _, quorumErr := coreupdate.VerifyPatroniQuorum(ctx, members, 0); quorumErr != nil {
			return fmt.Errorf("Core HA Patroni quorum not verified: %w", quorumErr)
		}
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
