package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestFullDestroyDiscoversProtectedCoreOnlyTarget(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const name = "orphan-core-only"
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	noop := func(context.Context, coreinstallation.State) error { return nil }
	_, err = coreinstallation.Run(context.Background(), runtimeRoot, coreinstallation.Spec{
		Target: name, Runtime: "docker", MachineRole: coreinstallation.Development,
	}, coreinstallation.Steps{
		Preflight: noop, SQL: noop, Secrets: noop, Verify: noop,
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://issuer.example.test", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	owned, err := deployment.ResolveDataRootTarget(name)
	if err != nil || owned.Name != name || owned.RuntimeProvider != "docker" || owned.AccessProvider != "local" {
		t.Fatalf("protected owner not resolved: %#v, %v", owned, err)
	}
	discovered, results := discoverFullDestroyTargets()
	for _, result := range results {
		if result.Target == name && result.Status == "FAILED" {
			t.Fatalf("valid owner rejected: %#v", result)
		}
	}
	for _, target := range discovered {
		if target.Name == name && target.RuntimeProvider == "docker" {
			return
		}
	}
	t.Fatalf("full destroy did not discover owned Core target: %#v", discovered)
}

func TestFullDestroyRejectsForeignCoreDataRoot(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const name = "foreign-core"
	root, err := deployment.TargetStateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime", "installation.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"owner\":\"foreign\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := deployment.ResolveDataRootTarget(name); err == nil {
		t.Fatal("foreign installation was accepted")
	}
	targets, results := discoverFullDestroyTargets()
	found := false
	for _, result := range results {
		if result.Target == name && result.Status == "FAILED" && result.Resource == "installation-owner" &&
			strings.Contains(result.Detail, "preserved") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing fail-closed foreign ownership result: %#v", results)
	}
	for _, target := range targets {
		if target.Name == name && target.RuntimeProvider != "" {
			t.Fatalf("foreign Core was assigned a runtime: %#v", target)
		}
	}
}
