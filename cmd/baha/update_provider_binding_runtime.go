package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// A reviewed adjacent minor transition requires an offline single-member
// migration and its verified SQL/configuration backup. HA remains patch-only.
func (o *coreNativeRuntimeOps) keycloakUpgradePath(ctx context.Context, from, to string) error {
	if !o.core.HA && from == "26.7.5" && to == "26.8.0" {
		return nil
	}
	return patchProviderUpgradePath(ctx, from, to)
}

func (o *coreNativeRuntimeOps) stopSelected(ctx context.Context, files bhruntime.Files, services ...string) error {
	environment, err := bhruntime.RuntimeEnvironment(files)
	if err != nil {
		return err
	}
	return o.runtime.StopProjectFilesSelected(ctx, files.Project, filepath.Dir(files.Compose), environment, services, files.Compose)
}

func providerMemberMutations(delta coreupdate.Delta, services []string) map[string]coreupdate.Delta {
	mutations := make(map[string]coreupdate.Delta, len(services))
	for _, service := range services {
		if strings.TrimSpace(service) == "" {
			continue
		}
		memberDelta := delta
		memberDelta.Installed.Instance = service
		mutations[service] = memberDelta
	}
	return mutations
}

func (o *coreNativeRuntimeOps) waitOpenBaoRollingMember(ctx context.Context, service, expectedVersion, expectedDigest string) error {
	if service == "" || expectedVersion == "" || expectedDigest == "" {
		return errors.New("OpenBao rolling readiness requires member, version and pinned digest")
	}
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	const statusScript = `rc=0
BAO_ADDR=https://127.0.0.1:8200 bao status -format=json || rc=$?
if [ "$rc" -eq 0 ] || [ "$rc" -eq 2 ]; then exit 0; fi
exit "$rc"`
	var lastErr error
	for {
		image, imageErr := o.runtime.ProjectServiceImageIdentity(bounded, o.core.Project, service)
		if imageErr == nil {
			digest := image.Digest
			if index := strings.Index(digest, "@sha256:"); index >= 0 {
				digest = digest[index+1:]
			}
			if digest == expectedDigest {
				output, probeErr := o.runtime.ExecProject(bounded, o.core.Project, o.core.Compose, o.core.Env, service, "sh", "-c", statusScript)
				if probeErr == nil {
					var state struct {
						Version     string `json:"version"`
						Initialized bool   `json:"initialized"`
						Sealed      bool   `json:"sealed"`
					}
					if jsonErr := json.Unmarshal([]byte(output), &state); jsonErr == nil &&
						state.Version == expectedVersion && state.Initialized && !state.Sealed {
						return nil
					} else if jsonErr != nil {
						lastErr = jsonErr
					} else {
						lastErr = fmt.Errorf("member %s state version=%s initialized=%t sealed=%t", service, state.Version, state.Initialized, state.Sealed)
					}
				} else {
					lastErr = probeErr
				}
			} else {
				lastErr = fmt.Errorf("member %s digest %s does not match target", service, digest)
			}
		} else {
			lastErr = imageErr
		}
		select {
		case <-bounded.Done():
			return fmt.Errorf("OpenBao rolling member %s did not become healthy: %v: %w", service, lastErr, bounded.Err())
		case <-ticker.C:
		}
	}
}

func (o *coreNativeRuntimeOps) waitKeycloakRollingMember(ctx context.Context, service, expectedDigest string) error {
	if service == "" || expectedDigest == "" {
		return errors.New("Keycloak rolling readiness requires member and pinned digest")
	}
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		containers, err := o.runtime.ListRuntimeContainers(bounded)
		if err == nil {
			found := false
			for _, container := range containers {
				if container.Project != o.identity.Project || container.Service != service {
					continue
				}
				found = true
				if container.Running && !strings.EqualFold(container.Health, "unhealthy") {
					image, imageErr := o.runtime.ProjectServiceImageIdentity(bounded, o.identity.Project, service)
					if imageErr == nil {
						digest := image.Digest
						if index := strings.Index(digest, "@sha256:"); index >= 0 {
							digest = digest[index+1:]
						}
						if digest == expectedDigest {
							return nil
						}
						lastErr = fmt.Errorf("member %s digest %s does not match target", service, digest)
					} else {
						lastErr = imageErr
					}
				} else {
					lastErr = fmt.Errorf("member %s is not running/healthy", service)
				}
			}
			if !found {
				lastErr = fmt.Errorf("member %s is not present in owned project", service)
			}
		} else {
			lastErr = err
		}
		select {
		case <-bounded.Done():
			return fmt.Errorf("Keycloak rolling member %s did not become ready: %v: %w", service, lastErr, bounded.Err())
		case <-ticker.C:
		}
	}
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
	keycloakOperatorPassword := identityEnv["BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD"]
	if keycloakUser == "" || keycloakPassword == "" || keycloakDatabase == "" || keycloakOperatorPassword == "" || credentials.PostgresInternalUser == "" || credentials.PostgresInternalPassword == "" {
		return coreupdate.BoundProviderTransaction{}, errors.New("Keycloak protected SQL credentials are incomplete")
	}

	backupDir := filepath.Join(journalDir, "provider-native")
	receiptDir := filepath.Join(journalDir, "provider-receipts")
	if err := os.MkdirAll(receiptDir, 0o700); err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	if err := os.Chmod(receiptDir, 0o700); err != nil {
		return coreupdate.BoundProviderTransaction{}, err
	}
	openBaoBackup := providerSQLBackupSpec{
		Runtime: o.runtime, Project: o.core.Project, Compose: o.core.Compose, Env: o.core.Env,
		Client: "postgres-admin", Host: "postgres", CAFile: "/run/baseharbor/postgres-ca/ca.pem",
		User: credentials.OpenBaoDBUser, Password: credentials.OpenBaoDBPassword, Database: "openbao",
		RestoreUser: credentials.PostgresInternalUser, RestorePassword: credentials.PostgresInternalPassword,
		Directory: backupDir, Name: "openbao", InstallationID: o.installation, Target: o.target, Transaction: o.release, Provider: providerupgrade.ProviderOpenBao,
		ConfigPaths: []string{o.core.Env, platformopenbao.AdminCredentialsPath(o.core), filepath.Join(filepath.Dir(o.core.Compose), "providers", "openbao", "runtime", "openbao.hcl")},
	}
	keycloakBackup := providerSQLBackupSpec{
		Runtime: o.runtime, Project: o.identity.Project, Compose: o.identity.Compose, Env: o.identity.Env,
		Client: "keycloak-db-init", Host: "keycloak-db", CAFile: "/run/baseharbor/db-tls/ca.pem",
		User: keycloakUser, Password: keycloakPassword, Database: keycloakDatabase,
		RestoreUser: "postgres", RestorePassword: keycloakOperatorPassword,
		Directory: backupDir, Name: "keycloak", InstallationID: o.installation, Target: o.target, Transaction: o.release, Provider: providerupgrade.ProviderKeycloak,
		ConfigPaths: []string{o.identity.Env, o.identity.Compose},
	}
	openBaoCompose := coreupdate.ComposeCheckpoint{Path: o.core.Compose, Directory: filepath.Join(journalDir, "compose-backups")}
	keycloakCompose := coreupdate.ComposeCheckpoint{Path: o.identity.Compose, Directory: filepath.Join(journalDir, "compose-backups")}
	openBaoMutations := providerMemberMutations(openBaoDelta, o.core.OpenBaoMembers())
	keycloakServices := []string{keycloakDelta.Installed.Instance}
	if o.core.HA {
		keycloakServices = []string{"keycloak-1", "keycloak-2", "keycloak-3"}
	}
	keycloakMutations := providerMemberMutations(keycloakDelta, keycloakServices)

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
			if !o.core.HA {
				if err := o.ReconcilePinned(ctx, openBaoDelta); err != nil {
					return err
				}
				return waitForOpenBaoExecReady(ctx, o.runtime, o.core)
			}
			environment, err := bhruntime.RuntimeEnvironment(o.core)
			if err != nil {
				return err
			}
			for _, member := range o.core.OpenBaoMembers() {
				if err := o.runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, o.core.Project, filepath.Dir(o.core.Compose),
					environment, []string{member}, o.core.Compose); err != nil {
					return fmt.Errorf("roll OpenBao member %s: %w", member, err)
				}
				// OpenBao may restart sealed. Unseal each restarted member before
				// waiting for the unsealed-version readiness gate; deferring
				// unseal until after the full roll would deadlock on a sealed node.
				recoveryPath, _, err := resolveTargetRecoveryFile(ctx, "")
				if err != nil {
					return err
				}
				if err := platformopenbao.Unseal(ctx, o.runtime, o.core, recoveryPath); err != nil {
					return fmt.Errorf("unseal OpenBao after rolling member %s: %w", member, err)
				}
				if err := o.waitOpenBaoRollingMember(ctx, member, openBaoDelta.Desired.Version, openBaoDelta.Desired.Digest); err != nil {
					return err
				}
			}
			return nil
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
			if err := waitForOpenBaoExecReady(ctx, o.runtime, o.core); err != nil {
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
		Compatibility: o.keycloakUpgradePath,
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
		StopHA: func(context.Context) error {
			return errors.New("UNSUPPORTED: non-rolling Keycloak HA replacement is not admitted")
		},
		ApplyHA: func(context.Context, string, string, string) error {
			return errors.New("UNSUPPORTED: non-rolling Keycloak HA replacement is not admitted")
		},
		ReplaceMember: func(ctx context.Context, member, version, image, digest string) error {
			if !o.core.HA {
				return errors.New("Keycloak rolling member replacement requires HA topology")
			}
			if version != keycloakDelta.Desired.Version || image != keycloakDelta.Desired.Image || digest != keycloakDelta.Desired.Digest {
				return errors.New("Keycloak rolling target differs from journaled Core delta")
			}
			allowed := false
			for _, expected := range keycloakServices {
				if member == expected {
					allowed = true
					break
				}
			}
			if !allowed {
				return errors.New("Keycloak rolling member is not owned by the managed HA topology")
			}
			if err := keycloakCompose.Stage(keycloakMutations); err != nil {
				return err
			}
			environment, err := bhruntime.RuntimeEnvironment(identityRuntime)
			if err != nil {
				return err
			}
			return o.runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, identityRuntime.Project, filepath.Dir(identityRuntime.Compose),
				environment, []string{member}, identityRuntime.Compose)
		},
		WaitMember: func(ctx context.Context, member string) error {
			return o.waitKeycloakRollingMember(ctx, member, keycloakDelta.Desired.Digest)
		},
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
			if err := o.stopSelected(ctx, identityRuntime, keycloakServices...); err != nil {
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
				providerupgrade.ProviderOpenBao:  {Project: o.core.Project, Service: openBaoDelta.Installed.Instance, Compose: o.core.Compose, OriginalImage: openBaoDelta.Installed.Image, OriginalDigest: openBaoDelta.Installed.Digest},
				providerupgrade.ProviderKeycloak: {Project: o.identity.Project, Service: keycloakDelta.Installed.Instance, Compose: o.identity.Compose, OriginalImage: keycloakDelta.Installed.Image, OriginalDigest: keycloakDelta.Installed.Digest},
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
