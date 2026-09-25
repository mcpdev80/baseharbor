package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTargetPrecedence(t *testing.T) {
	cfg := Config{
		Version: ConfigVersion,
		DefaultTarget: "default",
		Access: map[string]AccessDefinition{
			"a": {Provider: "docker", Reference: "local"},
		},
		Targets: map[string]TargetDefinition{
			"default": {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "a"}},
			"env":     {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "a"}},
			"flag":    {Runtime: RuntimeDefinition{Provider: "docker"}, Access: TargetAccess{Reference: "a"}},
		},
	}
	target, err := cfg.ResolveTarget("flag", "env")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "flag" {
		t.Fatalf("explicit target = %q", target.Name)
	}
	target, err = cfg.ResolveTarget("", "env")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "env" {
		t.Fatalf("activated target = %q", target.Name)
	}
	target, err = cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "default" {
		t.Fatalf("default target = %q", target.Name)
	}
}

func TestTargetStateRootIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", root)
	a, err := TargetStateRoot("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	b, err := TargetStateRoot("podman-dev")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("target state roots collide")
	}
	if want := filepath.Join(root, "baseharbor", "targets", "docker-dev"); a != want {
		t.Fatalf("root = %q, want %q", a, want)
	}
}

func TestDeploymentRegistryIndependentFromCWD(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	cwd := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	record := DeploymentRecord{
		Identity: DeploymentIdentity{Target: "docker-dev", Application: "demo", Environment: "dev"},
		Applied: AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	got, err := ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Identity.Application != "demo" {
		t.Fatalf("unexpected deployments: %#v", got)
	}
}

func TestSameApplicationEnvironmentAcrossTargets(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, target := range []string{"docker-dev", "podman-dev"} {
		if err := SaveDeploymentRecord(DeploymentRecord{
			Identity: DeploymentIdentity{Target: target, Application: "demo", Environment: "dev"},
			Applied: AppliedDeployment{RuntimeProvider: target},
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListAllDeployments()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("deployments = %d, want 2", len(got))
	}
}
