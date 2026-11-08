package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// configureTwoTestTargets writes an isolated two-Target configuration with no
// default and no persisted selection, so any Target choice must come from an
// explicit --target override or BASEHARBOR_TARGET. The caller's cwd is moved
// into an empty temporary repository directory so repository discovery can
// never resolve an application outside the selected Target plane.
func configureTwoTestTargets(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	cfg := deployment.Config{Version: deployment.ConfigVersion, Targets: map[string]deployment.TargetDefinition{
		"iso-a": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
		"iso-b": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
	}, Access: map[string]deployment.AccessDefinition{"local": {Provider: "local", Reference: "local"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// TestAppStatusHonorsEnvironmentTargetThroughProductionCLI routes the full
// production CLI path (runWithIO: global option parsing, context plumbing,
// command dispatch, effectiveTarget) and proves BASEHARBOR_TARGET selects the
// Target on a production lifecycle command, not only inside a test fixture.
// Mutation witness: replacing the os.Getenv reader in cmd/baha/target.go
// effectiveTarget with "" makes this test fail verbatim with "multiple
// deployment targets configured", so it pins the production read path.
func TestAppStatusHonorsEnvironmentTargetThroughProductionCLI(t *testing.T) {
	configureTwoTestTargets(t)
	if persisted, err := readPersistedTarget(); err != nil || persisted != "" {
		t.Fatalf("setup requires no persisted target: %q %v", persisted, err)
	}
	t.Setenv("BASEHARBOR_TARGET", "iso-b")
	var out, errOut bytes.Buffer
	statusErr := runWithIO(context.Background(), []string{"app", "status", "--json"}, &out, &errOut)
	if statusErr != nil && !strings.Contains(statusErr.Error(), "no application deployment could be resolved") {
		t.Fatalf("app status failed outside target selection: %v\n%s", statusErr, out.String())
	}
	// Without the environment reader the same invocation fails closed with the
	// multiple-targets conflict from selectedTargetName; assert the exact
	// production error identity is not present, which pins the env read.
	if strings.Contains(out.String(), "multiple deployment targets configured") {
		t.Fatalf("BASEHARBOR_TARGET was ignored on the app status production path: %s", out.String())
	}
}

// TestGlobalTargetOverrideWinsOverEnvironmentThroughProductionCLI asserts the
// documented precedence --target > BASEHARBOR_TARGET > persisted on the same
// production CLI path, using the read-only control-plane status command whose
// JSON report names the selected Target.
func TestGlobalTargetOverrideWinsOverEnvironmentThroughProductionCLI(t *testing.T) {
	configureTwoTestTargets(t)
	t.Setenv("BASEHARBOR_TARGET", "iso-b")
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"--target", "iso-a", "status", "--json"}, &out, &out); err != nil {
		t.Fatalf("status --json failed: %v\n%s", err, out.String())
	}
	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("status --json output: %v\n%s", err, out.String())
	}
	if report["target"] != "iso-a" {
		t.Fatalf("explicit --target did not win over BASEHARBOR_TARGET: %v", report["target"])
	}
}

// TestEnvironmentTargetSelectionOriginThroughProductionCLI proves the
// selection-origin reporter reads BASEHARBOR_TARGET on the production path and
// that the environment selection drives target state roots apart.
func TestEnvironmentTargetSelectionOriginThroughProductionCLI(t *testing.T) {
	configureTwoTestTargets(t)
	t.Setenv("BASEHARBOR_TARGET", "iso-b")
	target, err := effectiveTarget(context.Background())
	if err != nil {
		t.Fatalf("effectiveTarget: %v", err)
	}
	if target.Name != "iso-b" {
		t.Fatalf("BASEHARBOR_TARGET=iso-b selected %q", target.Name)
	}
	if origin := targetSelectionOrigin(context.Background()); origin != "environment" {
		t.Fatalf("selection origin: %s", origin)
	}
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if root != filepath.Join(os.Getenv("XDG_DATA_HOME"), "baseharbor", "targets", "iso-b") {
		t.Fatalf("environment target did not isolate the state root: %s", root)
	}
}
