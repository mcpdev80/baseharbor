package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestFullDestroyDryRunDoesNotMutateState(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	targetRoot, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(targetRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: true})
	var out bytes.Buffer
	err = runtimeDestroyCommand(ctx, []string{"-o", "json"}, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(targetRoot); err != nil {
		t.Fatalf("dry-run mutated target state: %v", err)
	}
	var report destroyReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("root destroy did not return the installation preview: %v\n%s", err, out.String())
	}
	if !report.All || !report.Preview {
		t.Fatalf("bare baha destroy did not select installation scope: %#v", report)
	}
}

func TestFullDestroyRemotePhaseExcludesOnlyRemoteTargetsAndKeepsLocalTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			"workstation": {Provider: "local", Reference: "workstation"},
			"core":        {Provider: "baseharbor-core", Reference: "core"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"dev-docker-1": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "core"}},
			"local":        {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "workstation"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	root, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"dev-docker-1", "local"} {
		if err := os.MkdirAll(filepath.Join(root, "targets", target), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "remote-connections", "dev-docker-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "remote-connections", "dev-docker-1", "profile.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	allTargets, _ := discoverFullDestroyTargets()
	if len(allTargets) != 2 {
		t.Fatalf("unfiltered inventory = %#v, want configured remote and local state Target", allTargets)
	}
	localTargets, discovery := discoverFullDestroyTargets(withFullDestroyExcludeRemoteTargets(context.Background()))
	if countFullDestroyBlockers(discovery) != 0 || len(localTargets) != 1 || localTargets[0].Name != "local" || localTargets[0].AccessProvider != "local" {
		t.Fatalf("local-phase inventory = %#v discovery=%#v; want only native local Target", localTargets, discovery)
	}
}

func TestFullDestroyRefusesMultipleRemoteCoresBeforeMutation(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	cfg := deployment.Config{
		Version: deployment.ConfigVersion,
		Access: map[string]deployment.AccessDefinition{
			"core-a": {Provider: "baseharbor-core", Reference: "core-a"},
			"core-b": {Provider: "baseharbor-core", Reference: "core-b"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"remote-a": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "core-a"}},
			"remote-b": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "core-b"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	dataRoot, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataRoot, "targets", "remote-a"), 0o700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	handled, err := destroyConnectedRemoteInstallation(context.Background(), true, &out, &out)
	if !handled || err == nil || !strings.Contains(err.Error(), "multiple remote Core installations") {
		t.Fatalf("multiple remote Cores result: handled=%t err=%v output=%s", handled, err, out.String())
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "targets", "remote-a")); err != nil {
		t.Fatalf("Target state was removed before refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configHome, "baseharbor", "config.yaml")); err != nil {
		t.Fatalf("Target config was removed before refusal: %v", err)
	}
}

func TestFullDestroyRemovesOwnedLocalStateButPreservesSourceRepository(t *testing.T) {
	configHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	targetRoot, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(targetRoot, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}

	sourceRoot := filepath.Join(t.TempDir(), "source-repository")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceMarker := filepath.Join(sourceRoot, "baseharbor.yaml")
	if err := os.WriteFile(sourceMarker, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runtimeDestroyAll(currentInstallationDestroyConsent(t), []string{"--all", "--yes"}, &out, &out); err != nil {
		t.Fatal(err)
	}

	dataRoot, err := deployment.DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("BaseHarbor data root still exists or stat failed: %v", err)
	}
	configPath, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(configPath)); !os.IsNotExist(err) {
		t.Fatalf("BaseHarbor config root still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(sourceMarker); err != nil {
		t.Fatalf("source repository was modified by full destroy: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Cleanup report")) {
		t.Fatalf("expected cleanup report, got:\n%s", out.String())
	}
}

func TestCountFullDestroyBlockersDistinguishesInformationalSkip(t *testing.T) {
	results := []fullDestroyResult{
		{Status: "REMOVED", Resource: "ok"},
		{Status: "SKIPPED", Resource: "runtime-provider", Detail: "runtime provider cannot be inferred; only local state can be removed safely"},
	}
	if got := countFullDestroyBlockers(results); got != 0 {
		t.Fatalf("informational skip blockers = %d, want 0", got)
	}

	results = append(results,
		fullDestroyResult{Status: "FAILED", Resource: "data-providers", Detail: "active endpoints"},
		fullDestroyResult{Status: "SKIPPED", Resource: "target-state", Detail: "preserved because runtime cleanup could not be verified"},
	)
	if got := countFullDestroyBlockers(results); got != 2 {
		t.Fatalf("cleanup blockers = %d, want 2", got)
	}
}

func TestTargetOwnedRuntimeContainersFiltersOnlyTargetProjects(t *testing.T) {
	containers := []bhruntime.RuntimeContainer{
		{Name: "shared", Project: "bh-local-shared", Service: "postgres"},
		{Name: "app", Project: "bh-local-demo-dev", Service: "demo-app"},
		{Name: "other", Project: "bh-other-demo-dev", Service: "demo-app"},
		{Name: "foreign", Project: "unrelated", Service: "x"},
	}
	got := targetOwnedRuntimeContainers("local", containers)
	if len(got) != 2 {
		t.Fatalf("target-owned containers = %#v, want 2", got)
	}
	if got[0].Project != "bh-local-demo-dev" || got[1].Project != "bh-local-shared" {
		t.Fatalf("unexpected target-owned ordering/content: %#v", got)
	}
}

func TestFullDestroyPreservesGlobalOwnershipOnHostCAFailure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "runtime-state"))
	root, err := deployment.TargetStateRoot("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	original := removeHostTrustForFullDestroy
	removeHostTrustForFullDestroy = func(_ context.Context, dir string) (hosttrust.RemovalResult, error) {
		expected, err := bhruntime.DataDir("")
		if err != nil {
			t.Fatal(err)
		}
		if dir != expected {
			t.Fatalf("destroy looked for trust state in %q, expected global %q", dir, expected)
		}
		return hosttrust.RemovalResult{Preserved: []hosttrust.AnchorRecord{{Fingerprint: strings.Repeat("a", 64), Path: "/usr/local/share/ca-certificates/baseharbor-aaaaaaaaaaaaaaaa.crt"}}}, errors.New("recorded CA mismatch")
	}
	defer func() { removeHostTrustForFullDestroy = original }()
	var out bytes.Buffer
	err = runtimeDestroyAll(currentInstallationDestroyConsent(t), []string{"--all", "--yes"}, &out, &out)
	if err == nil {
		t.Fatalf("unverified anchor unexpectedly permitted destroy: %s", out.String())
	}
	if !strings.Contains(out.String(), "PRESERVED host CA "+strings.Repeat("a", 64)) {
		t.Fatalf("missing exact preserved anchor warning: %s", out.String())
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("destroy discarded ownership evidence: %v", err)
	}
}

func TestFullDestroyJSONReportsPreservedCAOnFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "runtime-state"))
	original := removeHostTrustForFullDestroy
	removeHostTrustForFullDestroy = func(_ context.Context, _ string) (hosttrust.RemovalResult, error) {
		return hosttrust.RemovalResult{Preserved: []hosttrust.AnchorRecord{{Fingerprint: strings.Repeat("f", 64), Path: "/test/operator-owned.crt"}}}, errors.New("fingerprint changed")
	}
	defer func() { removeHostTrustForFullDestroy = original }()
	var out bytes.Buffer
	err := runtimeDestroyCommand(currentInstallationDestroyConsent(t), []string{"--all", "--yes", "--json"}, &out, &out)
	if err == nil {
		t.Fatal("unsafe destroy reported success")
	}
	if !strings.Contains(out.String(), "\"PRESERVED\"") || !strings.Contains(out.String(), "operator-owned.crt") {
		t.Fatalf("JSON did not report preserved CA: %s", out.String())
	}
}

func TestFailedRuntimeInventoryCannotRemoveOwnedHostTrust(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "runtime-state"))
	config, err := deployment.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("{broken-config"), 0600); err != nil {
		t.Fatal(err)
	}
	removed := false
	original := removeHostTrustForFullDestroy
	removeHostTrustForFullDestroy = func(context.Context, string) (hosttrust.RemovalResult, error) {
		removed = true
		return hosttrust.RemovalResult{}, nil
	}
	defer func() { removeHostTrustForFullDestroy = original }()
	var out bytes.Buffer
	err = runtimeDestroyAll(context.Background(), []string{"--all", "--yes"}, &out, &out)
	if removed {
		t.Fatalf("host CA removed despite failed installation inventory: %s", out.String())
	}
	if err == nil {
		t.Fatalf("corrupt registry did not block full destroy: %s", out.String())
	}
}
