package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
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

	ctx := localWorkloadScope(
		withLocalInvocation(operatorauth.WithEnforcement(context.Background()), "cli"),
		applicationBoundaryOperation("export"))
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

	ctx := localWorkloadScope(
		withLocalInvocation(operatorauth.WithEnforcement(context.Background()), "cli"),
		applicationBoundaryOperation("export"))
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
		[]byte("version: 1\napp:\n  name: mailflow\n  environment: dev\nworkload:\n  components:\n    - api\n"), 0o600); err != nil {
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
		t.Fatalf("export of secrets/openbao without a verified operator should fail closed; got stderr: %s", errOut.String())
	}
	if !strings.Contains(err.Error(), "authenticated BaseHarbor operator") {
		t.Fatalf("expected the operator-authentication denial for a managed-secret export without a verified operator; got: %v", err)
	}
}
