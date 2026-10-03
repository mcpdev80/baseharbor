package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestTargetListShowsImplicitLocalTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")

	var out bytes.Buffer
	cmd := targetCommand()
	list := cmd.Children[0]
	if err := list.Run(context.Background(), nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"local", "docker", "implicit", "effective"} {
		if !strings.Contains(text, want) {
			t.Fatalf("target list missing %q: %s", want, text)
		}
	}
}

func TestCreateTargetDoesNotBecomeDefaultWithoutFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	var out bytes.Buffer
	if err := createTarget(
		context.Background(),
		[]string{"kudo", "--runtime-provider", "docker", "--access", "local-docker", "--access-provider", "local", "--reference", "local", "--scope", "default"},
		&out,
		&out,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTarget != "" {
		t.Fatalf("default target = %q, want empty so implicit local remains effective", cfg.DefaultTarget)
	}
	resolved, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "local" {
		t.Fatalf("effective target = %q, want implicit local", resolved.Name)
	}
}

func TestCreateTargetSupportsRemoteNodeConnectorAccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	var out bytes.Buffer
	if err := createTarget(
		context.Background(),
		[]string{
			"edge-a",
			"--runtime-provider", "docker",
			"--access", "node-a",
			"--access-provider", "baseharbor-node-connector",
			"--reference", "node-a",
		},
		&out,
		&out,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	target := cfg.Targets["edge-a"]
	if target.Runtime.Provider != "docker" {
		t.Fatalf("runtime provider = %q", target.Runtime.Provider)
	}
	access := cfg.Access[target.Access.Reference]
	if access.Provider != "baseharbor-node-connector" || access.Reference != "node-a" {
		t.Fatalf("unexpected access definition: %#v", access)
	}
}

func TestTargetInspectionProjectsBuiltInAccessCapabilities(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "edge-a")

	var out bytes.Buffer
	if err := createTarget(
		context.Background(),
		[]string{
			"edge-a",
			"--runtime-provider", "docker",
			"--access", "node-a",
			"--access-provider", "baseharbor-node-connector",
			"--reference", "node-a",
		},
		&out,
		&out,
	); err != nil {
		t.Fatal(err)
	}

	result, err := collectTargetInspection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Target.AccessProvider != "baseharbor-node-connector" {
		t.Fatalf("access provider = %q", result.Target.AccessProvider)
	}
	if result.AccessCapabilities == nil {
		t.Fatal("built-in access capabilities missing")
	}
	if !result.AccessCapabilities.Remote ||
		!result.AccessCapabilities.Capabilities.Connect ||
		!result.AccessCapabilities.Capabilities.Stream ||
		!result.AccessCapabilities.Capabilities.PeerIdentity {
		t.Fatalf("unexpected access capabilities: %#v", result.AccessCapabilities)
	}
}
