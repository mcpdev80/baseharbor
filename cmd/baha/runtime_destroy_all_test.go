package main

import (
	"bytes"
	"context"
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
	if err := runtimeDestroyAll(ctx, []string{"--all"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(targetRoot); err != nil {
		t.Fatalf("dry-run mutated target state: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("No changes were made")) {
		t.Fatalf("expected dry-run guidance, got:\n%s", out.String())
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
	if err := runtimeDestroyAll(context.Background(), []string{"--all", "--yes"}, &out, &out); err != nil {
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
	err = runtimeDestroyAll(context.Background(), []string{"--all", "--yes"}, &out, &out)
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
	err := runtimeDestroyCommand(context.Background(), []string{"--all", "--yes", "--json"}, &out, &out)
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
