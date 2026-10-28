package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/health"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type bindingRuntimeFixture struct {
	bhruntime.RuntimeProvider
	engine bhruntime.DockerEngineObservation
	owned  []bhruntime.ProjectResource
}

func (p bindingRuntimeFixture) DockerEngine() *bhruntime.DockerEngineObservation { return &p.engine }
func (p bindingRuntimeFixture) ListOwnedProjectResources(context.Context, string) ([]bhruntime.ProjectResource, error) {
	return p.owned, nil
}

func TestDockerBindingPreventsCrossEngineMigrationAndCleanup(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "rootless-test", RuntimeProvider: "docker", AccessProvider: "local"}
	root, _ := targetRuntimeStateRoot(target)
	first := bindingRuntimeFixture{engine: bhruntime.DockerEngineObservation{Endpoint: "unix:///run/user/1000/docker.sock", Mode: "rootless", DaemonID: "original", Verified: true}}
	if err := validateTargetDockerBinding(context.Background(), target, first, true); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "docker-engine.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []bhruntime.DockerEngineObservation{
		{Endpoint: "unix:///var/run/docker.sock", Mode: "rootful", DaemonID: "system", Verified: true},
		{Endpoint: first.engine.Endpoint, Mode: "rootless", DaemonID: "replacement", Verified: true},
	} {
		if err := validateTargetDockerBinding(context.Background(), target, bindingRuntimeFixture{engine: changed}, true); err == nil {
			t.Fatal("changed engine admitted")
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, "docker-engine.json"))
	if string(before) != string(after) {
		t.Fatal("existing ownership changed")
	}
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	ctx := targetDockerEngineContext(context.Background(), target)
	// The provider resolver will receive the stored Target endpoint even when
	// CLI and long-running MCP processes inherit different active contexts.
	observed, err := bhruntime.ResolveDockerEngine(ctx, "nonexistent-docker-fixture")
	if err == nil || observed.Endpoint != first.engine.Endpoint {
		t.Fatalf("stored Target engine was not selected: %+v %v", observed, err)
	}
}

func TestUnboundLegacyCoreCannotBootstrapOnEmptyOtherEngine(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "legacy", RuntimeProvider: "docker", AccessProvider: "local"}
	root, _ := targetRuntimeStateRoot(target)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(path, []byte("protected-original-core"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := bindingRuntimeFixture{engine: bhruntime.DockerEngineObservation{Endpoint: "unix:///run/user/1000/docker.sock", Mode: "rootless", DaemonID: "empty-new-engine", Verified: true}}
	if err := validateTargetDockerBinding(context.Background(), target, provider, true); err == nil {
		t.Fatal("unproven legacy state adopted on empty engine")
	}
	if data, _ := os.ReadFile(path); string(data) != "protected-original-core" {
		t.Fatal("legacy Core modified")
	}
	if _, err := os.Stat(filepath.Join(root, "docker-engine.json")); !os.IsNotExist(err) {
		t.Fatal("legacy installation silently rebound")
	}
}

func TestDockerTargetIntentCLIAndMachineConfiguration(t *testing.T) {
	configureTestTarget(t)
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"target", "create", "cli-rootless", "--provider", "docker", "--access", "local-docker", "--reference", "local", "--docker-context", "rootless", "--docker-mode", "rootless"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := createTargetDefinition(context.Background(), machineTargetCreateInput{Name: "machine-rootless", RuntimeProvider: "docker", Access: "local-docker", Reference: "local", DockerContext: "rootless", DockerMode: "rootless"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := cfg.ResolveTarget("cli-rootless", "")
	if err != nil {
		t.Fatal(err)
	}
	machine, err := cfg.ResolveTarget("machine-rootless", "")
	if err != nil {
		t.Fatal(err)
	}
	if cli.DockerContext != "rootless" || machine.DockerContext != cli.DockerContext || machine.DockerMode != cli.DockerMode {
		t.Fatal("CLI and machine Target intent differ")
	}
	for _, input := range []machineTargetCreateInput{
		{Name: "bad-mode", RuntimeProvider: "docker", DockerMode: "automatic"},
		{Name: "conflicting-selection", RuntimeProvider: "docker", DockerEndpoint: "unix:///run/user/1000/docker.sock", DockerContext: "rootless"},
		{Name: "podman-docker-fields", RuntimeProvider: "podman", DockerMode: "rootful"},
	} {
		input.Access = "local-docker"
		input.Reference = "local"
		if _, err := createTargetDefinition(context.Background(), input); err == nil {
			t.Fatalf("invalid Target admitted: %s", input.Name)
		}
		after, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := after.Targets[input.Name]; ok {
			t.Fatal("invalid selection persisted")
		}
	}
}

func TestProtectedDockerBindingRejectsAmbiguousRecords(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	target := deployment.ResolvedTarget{Name: "protected-json", RuntimeProvider: "docker", AccessProvider: "local"}
	root, _ := targetRuntimeStateRoot(target)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	record := dockerEngineBinding{Schema: "baseharbor.docker-engine/v1", Target: target.Name, Observation: bhruntime.DockerEngineObservation{Endpoint: "unix:///run/user/1000/docker.sock", Mode: "rootless", DaemonID: "original", Verified: true}}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "docker-engine.json")
	for _, invalid := range [][]byte{append(append([]byte{}, data...), []byte(" {}")...), append([]byte(`{"unknown":true,`), data[1:]...)} {
		if err := os.WriteFile(path, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readDockerEngineBinding(path, target.Name); err == nil {
			t.Fatal("ambiguous protected record accepted")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(external, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}
	if _, err := readDockerEngineBinding(path, target.Name); err == nil {
		t.Fatal("symlink binding admitted")
	}
}

func TestHumanDoctorAlwaysDisplaysSelectedDockerEndpointAndMode(t *testing.T) {
	var out bytes.Buffer
	term := cli.NewTerminal(context.Background(), &out, &out)
	endpoint := "unix:///run/user/1000/docker.sock"
	if !renderControlPlaneDoctor(term, []health.Check{{Name: "Docker engine", OK: true, Message: endpoint + " (rootless; daemon original)"}}) {
		t.Fatal("healthy engine marked failed")
	}
	if !strings.Contains(out.String(), endpoint) || !strings.Contains(out.String(), "rootless") {
		t.Fatal("human doctor hid the actual endpoint or mode")
	}
}
