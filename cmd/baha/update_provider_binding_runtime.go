package main

import (
	"context"
	"errors"
	"fmt"
	"os"\n\t"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/providerbinding"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	keycloakadapter "github.com/mcpdev80/baseharbor/internal/providerupgrade/keycloak"
	baoAdapter "github.com/mcpdev80/baseharbor/internal/providerupgrade/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func corePlanOnly(plan coreupdate.Plan) (coreupdate.Plan, error) {
	if len(plan.Deltas) < 4 {
		return coreupdate.Plan{}, errors.New("incomplete managed Core/backing provider realization")
	}
	for _, d := range plan.Deltas[4:] {
		if d.Classification != coreupdate.NoChange {
			return coreupdate.Plan{}, fmt.Errorf("application-isolated provider %s requires its own verified migration before shared Core update: %s", d.Installed.Instance, d.Reason)
		}
	}
	return coreupdate.Plan{Release: plan.Release, Deltas: append([]coreupdate.Delta(nil), plan.Deltas[:4]...)}, nil
}

func providerDelta(plan coreupdate.Plan, kind coreupdate.ProviderKind) (coreupdate.Delta, error) {
	for _, d := range plan.Deltas {
		if d.Installed.Scope == "shared" && d.Installed.Kind == kind {
			return d, nil
		}
	}
	return coreupdate.Delta{}, fmt.Errorf("managed %s provider delta missing", kind)
}

func patchProviderUpgradePath(_ context.Context, from, to string) error {
	old, err := providerupgrade.ParseVersion(from)
	if err != nil {
		return err
	}
	next, err := providerupgrade.ParseVersion(to)
	if err != nil {
		return err
	}
	if !old.SameMinor(next) || old.Compare(next) >= 0 {
		return errors.New("UNSUPPORTED: provider upgrade requires a strictly newer patch within the same major/minor")
	}
	return nil
}

func (o *coreNativeRuntimeOps) stopSelected(ctx context.Context, files bhruntime.Files, services ...string) error {
	environment, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	return o.runtime.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), environment, []string{files.Compose}, services...)
}

func (o *coreNativeRuntimeOps) buildBoundProviderTransaction(ctx context.Context, plan coreupdate.Plan, journalDir, engine string) (coreupdate.BoundProviderTransaction, error) {
	openBaoDelta, err := providerDelta(plan, coreupdate.Secrets)
	if err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	keycloakDelta, err := providerDelta(plan, coreupdate.Identity)
	if err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	credentials, err := bhruntime.LoadControlPlaneCredentials(o.core)
	if err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	identityRuntime := bhruntime.Files{Project: o.identity.Project, Compose: o.identity.Compose, Env: o.identity.Env}
	identityEnv, err := bhruntime.RuntimeEnvironment(identityRuntime)
	if err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	keycloakUser := identityEnv["BASEHARBOR_KEYCLOAK_DB_USER"]
	keycloakPassword := identityEnv["BASEHARBOR_KEYCLOAK_DB_PASSWORD"]
	keycloakDatabase := identityEnv["BASEHARBOR_KEYCLOAK_DB_NAME"]
	if keycloakUser == "" || keycloakPassword == "" || keycloakDatabase == "" {
		return coreupdate.BoundProviderTransaction{}, errors.New("Keycloak protected SQL credentials are incomplete")
	}

	backupDir := filepath.Join(journalDir, "provider-native")\n\treceiptDir := filepath.Join(journalDir, "provider-receipts")\n\tif err := os.MkdirAll(receiptDir, 0o700); err != nil {\n\t\treturn coreupdate.BoundProviderTransaction{}, err\n\t}\n\tif err := os.Chmod(receiptDir, 0o700); err != nil {\n\t\treturn coreupdate.BoundProviderTransaction{}, err\n\t}
	openBaoBackup := providerSQLBackupSpec{
		Runtime: o.runtime, Project: o.core.Project, Compose: o.core.Compose, Env: o.core.Env,
		Client: "postgres-admin", Host: "postgres", CAFile: "/run/baseharbor/postgres-ca/ca.pem",
		User: credentials.OpenBaoDBUser, Password: credentials.OpenBaoDBPassword, Database: "openbao",
		Directory: backupDir, Name: "openbao", Provider: providerupgrade.ProviderOpenBao,
		ConfigPaths: []string{o.core.Env, platformopenbao.AdminCredentialsPath(o.core), filepath.Join(filepath.Dir(o.core.Compose), "providers", "openbao", "runtime", "openbao.hcl")},
	}
	keycloakBackup := providerSQLBackupSpec{
		Runtime: o.runtime, Project: o.identity.Project, Compose: o.identity.Compose, Env: o.identity.Env,
		Client: "keycloak-db-init", Host: "keycloak-db", CAFile: "/run/baseharbor/db-tls/ca.pem",
		User: keycloakUser, Password: keycloakPassword, Database: keycloakDatabase,
		Directory: backupDir, Name: "keycloak", Provider: providerupgrade.ProviderKeycloak,
		ConfigPaths: []string{o.identity.Env, o.identity.Compose},
	}
	openBaoCompose := coreupdate.ComposeCheckpoint{Path: o.core.Compose, Directory: filepath.Join(journalDir, "compose-backups")}
	keycloakCompose := coreupdate.ComposeCheckpoint{Path: o.identity.Compose, Directory: filepath.Join(journalDir, "compose-backups")}
	openBaoMutations := map[string]coreupdate.Delta{openBaoDelta.Installed.Instance: openBaoDelta}
	keycloakMutations := map[string]coreupdate.Delta{keycloakDelta.Installed.Instance: keycloakDelta}

	openBaoHooks := baoAdapter.RuntimeHooks{
		UpgradePath: patchProviderUpgradePath,
		Backup: func(ctx context.Context, version string) (providerupgrade.BackupRef, error) {
			return openBaoBackup.capture(ctx, version)
		},
		VerifyBackup: openBaoBackup.verify,
		Apply: func(ctx context.Context, version, image, digest string) error {
			if version != openBaoDelta.Desired.Version || image != openBaoDelta.Desired.Image || digest != openBaoDelta.Desired.Digest {
				return errors.New("OpenBao adapter target differs from journaled Core delta")
			}
			if err := openBaoCompose.Stage(openBaoMutations); err != nil {
				return err
			}
			return o.ReconcilePinned(ctx, openBaoDelta)
		},
		Unseal: func(ctx context.Context) error {
			recoveryPath, _, err := resolveTargetRecoveryFile(ctx, "")
			if err != nil {
				return err
			}
			return platformopenbao.Unseal(ctx, o.runtime, o.core, recoveryPath)
		},
		VerifyAuth: func(ctx context.Context) error {
			return platformopenbao.VerifyUpgradeManagerPolicyAndAppRole(ctx, o.runtime, o.core)
		},
		VerifyApps: o.verifyOwnedApplicationSecretScopes,
		Restore: func(ctx context.Context, ref providerupgrade.BackupRef, version string) error {
			if version != openBaoDelta.Installed.Version {
				return errors.New("OpenBao recovery version differs from journaled original")
			}
			if err := o.stopSelected(ctx, o.core, o.core.OpenBaoMembers()...); err != nil {
				return err
			}
			if err := openBaoBackup.restore(ctx, ref); err != nil {
				return err
			}
			if err := openBaoCompose.Restore(openBaoMutations); err != nil {
				return err
			}
			if err := o.ReconcileOriginal(ctx, openBaoDelta); err != nil {
				return err
			}
			recoveryPath, _, err := resolveTargetRecoveryFile(ctx, "")
			if err != nil {
				return err
			}
			return platformopenbao.Unseal(ctx, o.runtime, o.core, recoveryPath)
		},
	}

	keycloakHooks := keycloakadapter.CoreHooks{
		Inspect: func(ctx context.Context) (keycloakadapter.State, error) {
			return o.inspectNativeKeycloakMember(ctx, keycloakDelta.Installed.Instance)
		},
		Compatibility: patchProviderUpgradePath,
		Backup: func(ctx context.Context, version string) (providerupgrade.BackupRef, error) {
			return keycloakBackup.capture(ctx, version)
		},
		VerifyBackup: keycloakBackup.verify,
		ApplySingle: func(ctx context.Context, version, image, digest string) error {
			if version != keycloakDelta.Desired.Version || image != keycloakDelta.Desired.Image || digest != keycloakDelta.Desired.Digest {
				return errors.New("Keycloak adapter target differs from journaled Core delta")
			}
			if err := keycloakCompose.Stage(keycloakMutations); err != nil {
				return err
			}
			return o.ReconcilePinned(ctx, keycloakDelta)
		},
		StopHA: func(context.Context) error { return errors.New("UNSUPPORTED: Keycloak HA adapter requires rolling runtime binding") },
		ApplyHA: func(context.Context, string, string, string) error { return errors.New("UNSUPPORTED: Keycloak HA adapter requires rolling runtime binding") },
		ReplaceMember: func(context.Context, string, string, string, string) error { return errors.New("UNSUPPORTED: Keycloak HA adapter requires rolling runtime binding") },
		WaitMember: func(context.Context, string) error { return errors.New("UNSUPPORTED: Keycloak HA adapter requires rolling runtime binding") },
		WaitAll: func(ctx context.Context) error {
			_, err := o.inspectNativeKeycloakMember(ctx, keycloakDelta.Installed.Instance)
			return err
		},
		VerifySQL: o.verifyKeycloakBackingSQL,
		VerifyRealms: func(ctx context.Context) error {
			return identityprovider.VerifyCoreIdentity(ctx, o.dataDir, o.target, o.installation, o.issuer)
		},
		VerifyTokens: func(ctx context.Context) error {
			return identityprovider.VerifyCoreOperatorTokenFlow(ctx, o.dataDir, o.target, o.installation, o.issuer)
		},
		Restore: func(ctx context.Context, ref providerupgrade.BackupRef, version string) error {
			if version != keycloakDelta.Installed.Version {
				return errors.New("Keycloak recovery version differs from journaled original")
			}
			if err := o.stopSelected(ctx, identityRuntime, keycloakDelta.Installed.Instance); err != nil {
				return err
			}
			if err := keycloakBackup.restore(ctx, ref); err != nil {
				return err
			}
			if err := keycloakCompose.Restore(keycloakMutations); err != nil {
				return err
			}
			return o.ReconcileOriginal(ctx, keycloakDelta)
		},
	}
	registry, err := providerbinding.New(providerbinding.Dependencies{
		Inventory: &providerbinding.RuntimeBinding{Reader: o.runtime, Engine: engine,
			Sources: map[providerupgrade.Provider]providerbinding.ManagedSource{
				providerupgrade.ProviderOpenBao: {Project: o.core.Project, Service: openBaoDelta.Installed.Instance},
				providerupgrade.ProviderKeycloak: {Project: o.identity.Project, Service: keycloakDelta.Installed.Instance},
			}},
		OpenBaoExecutor: o.runtime, OpenBaoFiles: o.core, OpenBaoHooks: openBaoHooks,
		KeycloakDataDir: o.dataDir, KeycloakNamespace: o.target, KeycloakInstallationID: o.installation, KeycloakIssuer: o.issuer, KeycloakHooks: keycloakHooks,
	})
	if err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	// RuntimeBinding requires the concrete engine identity. It is already
	// verified by reconcileNativeCoreProviders; copy it into the registry source.
	_ = ctx
	return coreupdate.BoundProviderTransaction{Registry: registry, BackupDirectory: receiptDir, RecordExternal: o.Record}, nil
}
