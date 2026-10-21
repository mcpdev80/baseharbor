package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

func TestAppBackupRestoreCLIRealDisasterRecovery(t *testing.T) {
	if os.Getenv("BASEHARBOR_APP_BACKUP_ACCEPTANCE") != "true" {
		t.Skip("application backup CLI acceptance is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(root, "platform-state"))
	t.Setenv("BASEHARBOR_PROVIDER_CHANGE_CONSENT", "")
	configureAppBackupRuntimeTarget(t, filepath.Join(root, "config"), filepath.Join(root, "data"))
	// LIFO cleanup first executes the owned fixture's cleanup registered below,
	// then verifies the unrelated native volume before removing only that sentinel.
	registerBackupForeignVolumeSentinel(t, ctx)
	if err := runOwnedBackupCoreFixture(t, ctx,
		func(startCtx context.Context) error {
			return runtimeUpGuided(startCtx, strings.NewReader(""), io.Discard, runtimeUpOptions{Yes: true})
		},
		func(readyCtx context.Context) error {
			state, err := inspectManagedOpenBao(readyCtx)
			if err != nil {
				return err
			}
			if !state.Initialized || !state.Unsealed || !state.ManagerReady {
				return fmt.Errorf("guided Core OpenBao manager not ready: initialized=%t unsealed=%t manager=%t",
					state.Initialized, state.Unsealed, state.ManagerReady)
			}
			return nil
		},
		func(cleanupCtx context.Context) error {
			return approvedNativeFixtureCleanup(t, cleanupCtx, io.Discard)
		}); err != nil {
		t.Fatalf("guided Core start / authenticated OpenBao readiness: %v", err)
	}

	manifest := `version: 1
app:
  name: backup-cli
  environment: dev
services:
  sql:
    enabled: true
  cache:
    enabled: false
  secrets:
    enabled: true
`
	if err := os.WriteFile("baseharbor.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := application.ParseYAML(manifest); err != nil {
		t.Fatalf("fixture manifest must parse through the shipped capability parser: %v", err)
	}
	var output bytes.Buffer
	if err := runBackupFixtureWithConsent(ctx, "apply", func(callCtx context.Context) error {
		output.Reset()
		return runWithIO(callCtx, []string{"app", "apply"}, &output, &output)
	}); err != nil {
		t.Fatalf("app apply: %v\n%s", err, output.String())
	}

	store := application.DefaultStore()
	resolved, err := resolveApplication(ctx, store, nil, "backup acceptance")
	if err != nil {
		t.Fatal(err)
	}
	m := resolved.Manifest
	files, err := application.ExistingRuntimeFiles(resolved.Store, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backupFixtureSQL(ctx, resolved, true); err != nil {
		t.Fatalf("seed SQL through the actual consumer binding: %v", err)
	}
	secretService := applicationsecret.New(resolved.Store)
	if err := secretService.Set(ctx, m.Name, "API_TOKEN", []byte("provider-key-v1")); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	tokenBefore, err := os.ReadFile(application.RuntimeIdentityTokenPath(files))
	if err != nil {
		t.Fatal(err)
	}
	clientCertPath := filepath.Join(files.Bindings, "runtime-identity", "client-cert.pem")
	certBefore, err := os.ReadFile(clientCertPath)
	if err != nil {
		t.Fatal(err)
	}

	passwordPath := filepath.Join(root, "backup-password")
	if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(root, "backup-cli.bhbackup")
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "backup", "--password-file", passwordPath, "--output", backupPath}, &output, &output); err != nil {
		t.Fatalf("app backup: %v\n%s", err, output.String())
	}
	metadata, err := resolved.Store.LastBackup(m.Name)
	if err != nil {
		t.Fatalf("load recorded backup metadata: %v", err)
	}
	if metadata.Application != m.Name || metadata.Environment != m.Environment || metadata.CreatedAt.IsZero() || metadata.ArchivePath != backupPath || len(metadata.PostgresResources) != 1 || metadata.PostgresResources[0] != "default" || !metadata.IncludesSecrets {
		t.Fatalf("unexpected recorded backup metadata: %#v", metadata)
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "show"}, &output, &output); err != nil {
		t.Fatalf("app show after backup: %v\n%s", err, output.String())
	}
	showOutput := output.String()
	if !strings.Contains(showOutput, "Last backup") || !strings.Contains(showOutput, backupPath) || !strings.Contains(showOutput, "managed secrets      included") {
		t.Fatalf("app show did not report last backup metadata:\n%s", showOutput)
	}
	if strings.Contains(showOutput, "provider-key-v1") || strings.Contains(showOutput, "API_TOKEN") {
		t.Fatalf("app show exposed secret-bearing backup detail:\n%s", showOutput)
	}

	output.Reset()
	if err := runBackupFixtureWithConsent(ctx, "destroy", func(callCtx context.Context) error {
		output.Reset()
		return runWithIO(callCtx, []string{"app", "destroy", "--yes"}, &output, &output)
	}); err != nil {
		t.Fatalf("app destroy: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := runBackupFixtureWithConsent(ctx, "restore", func(callCtx context.Context) error {
		output.Reset()
		return runWithIO(callCtx, []string{"app", "restore", backupPath, "--password-file", passwordPath}, &output, &output)
	}); err != nil {
		t.Fatalf("app restore: %v\n%s", err, output.String())
	}
	restoreOutput := output.String()
	if !strings.Contains(restoreOutput, "was restored and verified") || !strings.Contains(restoreOutput, "Status: READY") {
		t.Fatalf("restore did not report READY after verification:\n%s", restoreOutput)
	}

	resolved, err = resolveApplication(ctx, application.DefaultStore(), nil, "backup acceptance verify")
	if err != nil {
		t.Fatal(err)
	}
	recoveryMetadata, err := resolved.Store.LastRecovery(resolved.Manifest.Name)
	if err != nil {
		t.Fatalf("load recorded recovery metadata: %v", err)
	}
	if recoveryMetadata.Application != resolved.Manifest.Name || recoveryMetadata.Environment != resolved.Manifest.Environment || recoveryMetadata.RestoredAt.IsZero() || recoveryMetadata.BackupCreatedAt != metadata.CreatedAt || recoveryMetadata.ArchivePath != backupPath {
		t.Fatalf("unexpected recorded recovery metadata: %#v", recoveryMetadata)
	}
	output.Reset()
	if err := runWithIO(ctx, []string{"app", "show"}, &output, &output); err != nil {
		t.Fatalf("app show after recovery: %v\n%s", err, output.String())
	}
	showOutput = output.String()
	if !strings.Contains(showOutput, "Last recovery") || !strings.Contains(showOutput, "verification         READY") || !strings.Contains(showOutput, backupPath) {
		t.Fatalf("app show did not report verified recovery metadata:\n%s", showOutput)
	}
	if strings.Contains(showOutput, "provider-key-v1") || strings.Contains(showOutput, "API_TOKEN") {
		t.Fatalf("app show exposed secret-bearing recovery detail:\n%s", showOutput)
	}

	files, err = application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	query, err := backupFixtureSQL(ctx, resolved, false)
	if err != nil {
		t.Fatalf("verify postgres: %v", err)
	}
	if string(bytes.TrimSpace([]byte(query))) != "survived" {
		t.Fatalf("restored postgres value = %q", query)
	}
	secretService = applicationsecret.New(resolved.Store)
	secretValue, err := secretService.Get(ctx, resolved.Manifest.Name, "API_TOKEN")
	if err != nil {
		t.Fatalf("verify restored secret: %v", err)
	}
	if !bytes.Equal(secretValue, []byte("provider-key-v1")) {
		t.Fatal("restored secret differs from the expected protected value")
	}
	tokenAfter, err := os.ReadFile(application.RuntimeIdentityTokenPath(files))
	if err != nil {
		t.Fatal(err)
	}
	certAfter, err := os.ReadFile(filepath.Join(files.Bindings, "runtime-identity", "client-cert.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(tokenBefore, tokenAfter) {
		t.Fatal("runtime token was replayed instead of regenerated")
	}
	if bytes.Equal(certBefore, certAfter) {
		t.Fatal("runtime mTLS client certificate was replayed instead of regenerated")
	}
}

func registerBackupForeignVolumeSentinel(t *testing.T, ctx context.Context) {
	t.Helper()
	target, err := effectiveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	project := "backup-foreign-" + application.MustNewApplicationID()
	resource := bhruntime.ProjectResource{Kind: "volume", Name: project + "-sentinel"}
	before, err := provider.InspectProjectResources(ctx, project, []bhruntime.ProjectResource{resource})
	if err != nil || len(before) != 0 {
		t.Fatal("foreign sentinel creation requires a verified absent resource")
	}
	if err := provider.EnsureOwnedVolume(ctx, project, resource.Name); err != nil {
		t.Fatal("create separately owned native volume sentinel", err)
	}
	type volumeIdentity struct {
		Name      string
		CreatedAt string
		Driver    string
		Labels    map[string]string
	}
	inspect := func(ctx context.Context) (volumeIdentity, error) {
		direct, ok := provider.(interface {
			DirectOutput(context.Context, ...string) (string, error)
		})
		if !ok {
			return volumeIdentity{}, fmt.Errorf("native runtime lacks bound volume metadata inspection")
		}
		data, err := direct.DirectOutput(ctx, "volume", "inspect", resource.Name)
		if err != nil {
			return volumeIdentity{}, fmt.Errorf("inspect foreign native sentinel failed")
		}
		var identities []volumeIdentity
		if err := json.Unmarshal([]byte(data), &identities); err != nil || len(identities) != 1 {
			return volumeIdentity{}, fmt.Errorf("foreign native sentinel metadata is not singular and valid")
		}
		identity := identities[0]
		if identity.Name != resource.Name || identity.CreatedAt == "" || identity.Driver == "" ||
			identity.Labels["com.docker.compose.project"] != project ||
			identity.Labels["io.podman.compose.project"] != project {
			return volumeIdentity{}, fmt.Errorf("foreign native sentinel identity or ownership is unverifiable")
		}
		return identity, nil
	}
	var original volumeIdentity
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, err := inspect(cleanupCtx)
		if err != nil || original.CreatedAt == "" || !reflect.DeepEqual(current, original) {
			t.Error("owned cleanup removed or changed the foreign native sentinel; replacement deletion refused")
			return
		}
		owned, err := provider.InspectProjectResources(cleanupCtx, project, []bhruntime.ProjectResource{resource})
		if err != nil || len(owned) != 1 || owned[0] != resource {
			t.Error("foreign native sentinel ownership changed; explicit sentinel cleanup refused")
			return
		}
		if err := provider.DestroyOwnedProjectResources(cleanupCtx, project, []bhruntime.ProjectResource{resource}); err != nil {
			t.Error("explicit removal of the test's separately owned volume failed", err)
			return
		}
		remaining, err := provider.InspectProjectResources(cleanupCtx, project, []bhruntime.ProjectResource{resource})
		if err != nil || len(remaining) != 0 {
			t.Error("explicit native sentinel removal could not be verified")
			return
		}
		t.Log("foreign native volume retained its exact identity after owned cleanup; separate sentinel removal verified")
	})
	original, err = inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

// Configure the real guided-up Target in the isolated test state. The native
// test runtime environment selects the runner, not the product default Target.
func configureAppBackupRuntimeTarget(t *testing.T, configRoot, dataRoot string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("BASEHARBOR_TARGET", "")
	kind, err := backupFixtureRuntimeKind()
	if err != nil {
		t.Fatalf("native backup fixture runtime selection: %v", err)
	}
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "backup-recovery-ci",
		Access: map[string]deployment.AccessDefinition{
			"local-backup-ci": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"backup-recovery-ci": {
				Runtime: deployment.RuntimeDefinition{Provider: string(kind)},
				Access:  deployment.TargetAccess{Reference: "local-backup-ci"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != cfg.DefaultTarget || target.RuntimeProvider != string(kind) || target.AccessProvider != "local" {
		t.Fatalf("native guided-up Target mismatch: got %q/%q/%q, want %q/%q/local",
			target.Name, target.RuntimeProvider, target.AccessProvider, cfg.DefaultTarget, kind)
	}
}
func backupFixtureRuntimeKind() (string, error) {
	kind, err := runtimecontract.ParseProviderKind(strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME")))
	if err != nil {
		return "", err
	}
	if string(kind) != "docker" && string(kind) != "podman" {
		return "", fmt.Errorf("unsupported native backup test runtime %q", kind)
	}
	return string(kind), nil
}

// runOwnedBackupCoreFixture registers the existing consent-checked owner
// before startup, including partial starts. Recovery never re-bootstraps an
// already initialized trust plane. Failure retains the original start error;
// cleanup is independently bounded and runs even after parent cancellation.
func runOwnedBackupCoreFixture(t *testing.T, ctx context.Context,
	start, ready, stop func(context.Context) error) error {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := stop(cleanupCtx); err != nil {
			t.Errorf("owned Core fixture cleanup: %v", err)
		}
	})
	if err := start(ctx); err != nil {
		return err
	}
	return ready(ctx)
}

// TestBackupFixtureManifestMatchesCapabilityContract keeps the disaster-recovery
// fixture manifest in lockstep with the shipped v1 capability grammar without
// starting any runtime. It fails closed if the manifest reverts to legacy
// postgres/redis service keys that the parser rejects.
func TestBackupFixtureManifestMatchesCapabilityContract(t *testing.T) {
	manifest := `version: 1
app:
  name: backup-cli
  environment: dev
services:
  sql:
    enabled: true
  cache:
    enabled: false
  secrets:
    enabled: true
`
	m, err := application.ParseYAML(manifest)
	if err != nil {
		t.Fatalf("fixture manifest must parse through the shipped capability parser: %v", err)
	}
	if m.Name != "backup-cli" || m.Environment != "dev" {
		t.Fatalf("unexpected parsed application identity: name=%q environment=%q", m.Name, m.Environment)
	}
	if !m.Services.SQL {
		t.Fatalf("fixture must request the sql capability")
	}
	if m.Services.Cache {
		t.Fatalf("fixture must not enable the cache capability")
	}
	if !m.Services.Secrets {
		t.Fatalf("fixture must request the secrets capability")
	}
}

// TestBackupFixtureManifestRejectsLegacyCapabilityKeys proves the shipped parser
// refuses the legacy postgres/redis service keys so the fixture can never drift
// back to them. No runtime is started.
func TestBackupFixtureManifestRejectsLegacyCapabilityKeys(t *testing.T) {
	legacy := `version: 1
app:
  name: backup-cli
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: true
`
	if _, err := application.ParseYAML(legacy); err == nil {
		t.Fatalf("parser must reject legacy postgres/redis service keys")
	}
}
