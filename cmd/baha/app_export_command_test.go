package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// app_export_command_test.go is the command-path assembly cell for
// 0428-065 (t-20261114-r28-cli2-app-export): it drives the export through the
// real rootCommand() tree — the exact registration line in
// cmd/baha/commands.go is the only wiring under test, so removing that line
// makes THIS cell red, not merely the generated documentation checks.

func TestAppExportCommandPathAssemblesProductionWiring(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)

	root := rootCommand()
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("BASEHARBOR_TARGET", "app-export-ci")
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "app-export-ci",
		Access: map[string]deployment.AccessDefinition{
			"local-app-export-ci": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"app-export-ci": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-app-export-ci"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	const appID = "11111111-1111-4111-8111-111111111111"
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath,
		[]byte("version: 1\napp:\n  name: mailflow\n  environment: dev\nworkload:\n  components:\n    - api\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := application.LoadManifestFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "mailflow" || manifest.Environment != "dev" {
		t.Fatalf("fixture manifest mismatch: %#v", manifest)
	}
	owner := application.Store{Root: filepath.Join(repo, ".baseharbor", "apps")}
	storeManifest := manifest
	storeManifest.ApplicationID = appID
	if _, err := owner.Create(storeManifest); err != nil {
		t.Fatalf("fixture application identity: %v", err)
	}

	local, err := devaccess.Configure("app-export-ci", "dev", "developer", "local-developer-secret-9kZp")
	if err != nil {
		t.Fatal(err)
	}
	credPath, err := devaccess.Path("app-export-ci", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(credPath); err != nil {
		t.Fatalf("developer credentials were not written to the command state path: %v", err)
	}

	passwordPath := filepath.Join(repo, "export-password.txt")
	const exportPassword = "correct horse battery staple"
	if err := os.WriteFile(passwordPath, []byte(exportPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(repo, "mailflow-export.bhexport")

	ctx := appExportFixtureContext("cli")
	var out, errOut bytes.Buffer
	if err := root.Execute(ctx, []string{
		"app", "export",
		"--include", "local.developer-credentials",
		"--password-file", passwordPath,
		"--output", outputPath,
	}, &out, &errOut); err != nil {
		t.Fatalf("baha app export through the command tree failed: %v (stderr: %s)", err, errOut.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("export output was not created by the command path: %v", err)
	}
	if info, err := os.Stat(outputPath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("export output is group/world accessible: %o", info.Mode().Perm())
	}

	openedManifest, entries, err := applicationbackup.OpenExport(data, []byte(exportPassword))
	if err != nil {
		t.Fatalf("OpenExport(correct password) error = %v", err)
	}
	if openedManifest.Application != "mailflow" || openedManifest.Environment != "dev" || openedManifest.ApplicationID != appID {
		t.Fatalf("export manifest mismatch: %#v", openedManifest)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (%+v)", len(entries), entries)
	}
	byName := map[string]string{}
	for _, e := range entries {
		byName[e.Name] = string(e.Data)
	}
	if byName["developer-access/username"] != local.Username || byName["developer-access/password"] != local.Password {
		t.Fatalf("exported credentials do not match the locally loaded developer credentials: %+v", byName)
	}

	if _, _, err := applicationbackup.OpenExport(data, []byte("wrong password")); err == nil {
		t.Fatal("OpenExport(wrong password) = nil error; command output readable without the password")
	}
	if _, _, err := applicationbackup.OpenExport(data, nil); err == nil {
		t.Fatal("OpenExport(nil) = nil error; command output readable without a password")
	}

	combined := out.String() + errOut.String()
	for _, leak := range []string{local.Password, exportPassword} {
		if strings.Contains(combined, leak) {
			t.Fatalf("command output leaks a secret through the command path: %q", leak)
		}
		var raw struct {
			Magic      string `json:"magic"`
			Ciphertext string `json:"ciphertext"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("export file is not a JSON envelope: %v", err)
		}
		if strings.Contains(raw.Ciphertext, leak) {
			t.Fatalf("export ciphertext contains plaintext %q", leak)
		}
	}
}

// TestAppExportFailsClosedWithoutSelection documents that an export with no
// selected source fails closed rather than writing an empty or silently
// defaulted envelope. This is the fail-closed-by-construction invariant for
// the selection surface.
func TestAppExportFailsClosedWithoutSelection(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)

	root := rootCommand()
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("BASEHARBOR_TARGET", "app-export-ci")
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "app-export-ci",
		Access: map[string]deployment.AccessDefinition{
			"local-app-export-ci": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"app-export-ci": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-app-export-ci"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath,
		[]byte("version: 1\napp:\n  name: mailflow\n  environment: dev\nworkload:\n  components:\n    - api\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	owner := application.Store{Root: filepath.Join(repo, ".baseharbor", "apps")}
	manifest, loadErr := application.LoadManifestFile(manifestPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	storeManifest := manifest
	storeManifest.ApplicationID = "11111111-1111-4111-8111-111111111111"
	if _, createErr := owner.Create(storeManifest); createErr != nil {
		t.Fatalf("fixture application identity: %v", createErr)
	}

	passwordPath := filepath.Join(repo, "export-password.txt")
	if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := appExportFixtureContext("cli")
	var out, errOut bytes.Buffer
	err := root.Execute(ctx, []string{
		"app", "export",
		"--password-file", passwordPath,
	}, &out, &errOut)
	if err == nil {
		t.Fatalf("export without selection should fail closed; got stderr: %s", errOut.String())
	}
	if !strings.Contains(err.Error(), "at least one") && !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected a fail-closed empty-selection error; got: %v", err)
	}
}

// TestAppExportCommandProducesCommand verifies the command node exists and is
// runnable, mirroring the non-tree assertion in the secure-profile test.
func TestAppExportCommandProducesCommand(t *testing.T) {
	cmd := appExportCommand(application.Store{})
	if cmd == nil || cmd.Name != "export" {
		t.Fatalf("appExportCommand does not produce the export command: %#v", cmd)
	}
	if cmd.Run == nil {
		t.Fatal("export command has no Run handler")
	}
}

// TestAppExportManagedSecretWithoutOperatorFailsClosed exercises the
// fail-closed human-authorization reveal seam for the managed-secret source:
// requesting secrets/openbao without a verified operator principal must fail
// closed with the operator-authentication denial, never returning a value or a
// silent fallback.
func TestAppExportManagedSecretWithoutOperatorFailsClosed(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)

	root := rootCommand()
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("BASEHARBOR_TARGET", "app-export-ci")
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "app-export-ci",
		Access: map[string]deployment.AccessDefinition{
			"local-app-export-ci": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"app-export-ci": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-app-export-ci"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	const appID = "11111111-1111-4111-8111-111111111111"
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath,
		[]byte("version: 1\napp:\n  name: mailflow\n  environment: dev\nworkload:\n  components:\n    - api\nservices:\n  secrets: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, loadErr := application.LoadManifestFile(manifestPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	storeManifest := manifest
	storeManifest.ApplicationID = appID
	owner := application.Store{Root: filepath.Join(repo, ".baseharbor", "apps")}
	if _, createErr := owner.Create(storeManifest); createErr != nil {
		t.Fatalf("fixture application identity: %v", createErr)
	}

	passwordPath := filepath.Join(repo, "export-password.txt")
	if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Honest negative: a context with enforcement enabled but NO verified
	// operator principal. The production export path resolves the application
	// through the managed-environment operator-auth boundary (during
	// resolution) BEFORE the secrets/openbao human-authorization reveal, so an
	// absent principal fails closed there. This is the fail-closed-by-construction
	// invariant for the managed-secret export surface: without a verified BaseHarbor
	// operator the command never reaches the reveal or the secret backend.
	ctx := localWorkloadScope(
		withLocalInvocation(operatorauth.WithEnforcement(context.Background()), "cli"),
		applicationBoundaryOperation("export"))
	var out, errOut bytes.Buffer
	err := root.Execute(ctx, []string{
		"app", "export",
		"--include", "secrets/openbao",
		"--password-file", passwordPath,
	}, &out, &errOut)
	if err == nil {
		t.Fatalf("managed-secret export without a verified operator should fail closed; got stderr: %s", errOut.String())
	}
	if strings.Contains(err.Error(), "Selected export sources") {
		t.Fatalf("managed-secret export without a verified operator should fail closed before selecting sources; got: %v", err)
	}
}

type appExportSecretReaderFake struct {
	listItems []applicationsecret.Metadata
	values    map[string][]byte
	listErr   error
	getErr    error
}

func (f *appExportSecretReaderFake) List(context.Context, string) ([]applicationsecret.Metadata, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]applicationsecret.Metadata(nil), f.listItems...), nil
}

func (f *appExportSecretReaderFake) GetMany(_ context.Context, _ string, keys []string) (map[string][]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	values := make(map[string][]byte, len(keys))
	for _, key := range keys {
		values[key] = append([]byte(nil), f.values[key]...)
	}
	return values, nil
}

func appExportAuthorizedTestContext() context.Context {
	ctx := operatorauth.WithEnforcement(context.Background())
	ctx = operatorauth.WithVerifiedPrincipal(ctx, &identity.Principal{
		Issuer:         "https://issuer.example",
		Subject:        "export-operator",
		Groups:         []string{"editor"},
		IssuerVerified: true,
	})
	return operatorauth.WithMembershipProvider(ctx, readGrantingMembershipProvider{})
}

// appExportFixtureContext composes the operator-authentication fixture that
// develop's managed-environment boundary demands before any CLI command (here
// the export command) reaches the production command tree: enforcement is
// enabled, a verified test-operator principal is installed, and a bounded
// membership provider answers the claim instead of the production Core-backed
// resolver (which cannot answer without a database). The pattern mirrors the
// operator-auth fixture the 0428-142 status-witness cell builds
// (commit 77d0415ef: persesMatrixCommandContext) and the readGrantingMembership
// provider used across the package. Providing this identity does not weaken the
// seam: the reveal path below (authorizeHumanReveal) still requires a human
// operator for the managed-secret source, so a non-human verified principal
// fails closed there with the operator-authentication denial.
func appExportFixtureContext(transport string) context.Context {
	ctx := operatorauth.WithEnforcement(context.Background())
	ctx = operatorauth.WithVerifiedPrincipal(ctx, &identity.Principal{
		Issuer:         "https://export-fixture.invalid",
		Subject:        "export-fixture-operator",
		Groups:         []string{"editor"},
		IssuerVerified: true,
	})
	ctx = operatorauth.WithMembershipProvider(ctx, &readGrantingMembershipProvider{})
	return localWorkloadScope(
		withLocalInvocation(ctx, transport),
		applicationBoundaryOperation("export"))
}

func appExportTestResolved() resolvedApplication {
	return resolvedApplication{
		Target: deployment.ResolvedTarget{Name: "export-target"},
		Manifest: application.Manifest{
			Version: 1, ApplicationID: "11111111-1111-4111-8111-111111111111",
			Name: "mailflow", Environment: "dev", Services: application.Services{Secrets: true},
		},
	}
}

func TestAppExportManagedSecretsAuthorizedSelectionProducesEncryptedEntry(t *testing.T) {
	const value = "managed-secret-sentinel-export-4f92c1"
	reader := &appExportSecretReaderFake{
		listItems: []applicationsecret.Metadata{{Name: "API_TOKEN", Present: true}},
		values:    map[string][]byte{"API_TOKEN": []byte(value)},
	}
	passwordPath := filepath.Join(t.TempDir(), "password")
	const password = "correct horse battery staple"
	if err := os.WriteFile(passwordPath, []byte(password+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "managed-export.bhexport")
	var out bytes.Buffer
	factory := func(context.Context, resolvedApplication) (appExportSecretReader, error) { return reader, nil }
	if err := executeResolvedApplicationExport(appExportAuthorizedTestContext(), appExportTestResolved(), []string{"secrets/openbao"}, outputPath, passwordPath, &out, factory); err != nil {
		t.Fatalf("authorized managed-secret export failed: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("export artifact missing: %v", err)
	}
	_, entries, err := applicationbackup.OpenExport(data, []byte(password))
	if err != nil {
		t.Fatalf("OpenExport() failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "secrets/openbao/API_TOKEN" || string(entries[0].Data) != value {
		t.Fatalf("managed secret export entries = %#v", entries)
	}
	if strings.Contains(out.String(), value) {
		t.Fatalf("managed secret leaked to command output: %q", value)
	}
	var envelope struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("export is not an encrypted JSON envelope: %v", err)
	}
	if strings.Contains(envelope.Ciphertext, value) {
		t.Fatalf("ciphertext contains managed secret in plaintext: %q", value)
	}
}

func TestAppExportManagedSecretBackendErrorsAreRedacted(t *testing.T) {
	const sentinel = "managed-secret-error-sentinel-53ab"
	for _, phase := range []string{"list", "read"} {
		t.Run(phase, func(t *testing.T) {
			reader := &appExportSecretReaderFake{listItems: []applicationsecret.Metadata{{Name: "API_TOKEN", Present: true}}}
			if phase == "list" {
				reader.listErr = errors.New(sentinel)
			} else {
				reader.getErr = errors.New(sentinel)
			}
			passwordPath := filepath.Join(t.TempDir(), "password")
			if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			factory := func(context.Context, resolvedApplication) (appExportSecretReader, error) { return reader, nil }
			err := executeResolvedApplicationExport(appExportAuthorizedTestContext(), appExportTestResolved(), []string{"secrets/openbao"}, filepath.Join(t.TempDir(), "export.bhexport"), passwordPath, &out, factory)
			if err == nil {
				t.Fatal("managed secret backend error was accepted")
			}
			if strings.Contains(err.Error()+out.String(), sentinel) {
				t.Fatalf("managed secret backend error leaked detail %q: %v", sentinel, err)
			}
		})
	}
}
