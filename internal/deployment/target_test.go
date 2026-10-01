package deployment

import (
	"os"
	"path/filepath"
	"testing"
)

const testApplicationID = "11111111-1111-4111-8111-111111111111"

func testDeploymentIdentity(t *testing.T, target, application, environment string) DeploymentIdentity {
	t.Helper()
	id, err := NewDeploymentIdentity(target, testApplicationID, application, environment)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestResolveTargetPrecedence(t *testing.T) {
	cfg := Config{
		Version:       ConfigVersion,
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
	for path, provider := range map[string]string{a: "docker", b: "podman"} {
		runtimeDir := filepath.Join(path, "runtime")
		if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runtimeDir, "provider"), []byte(provider), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dockerProvider, err := os.ReadFile(filepath.Join(a, "runtime", "provider"))
	if err != nil {
		t.Fatal(err)
	}
	podmanProvider, err := os.ReadFile(filepath.Join(b, "runtime", "provider"))
	if err != nil {
		t.Fatal(err)
	}
	if string(dockerProvider) != "docker" || string(podmanProvider) != "podman" {
		t.Fatalf("target runtime state crossed boundaries: docker=%q podman=%q", dockerProvider, podmanProvider)
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
		Identity: testDeploymentIdentity(t, "docker-dev", "demo", "dev"),
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
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

func TestLoadDeploymentRecordClassifiesMissingState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id := testDeploymentIdentity(t, "docker-dev", "demo", "dev")
	root, err := DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err = LoadDeploymentRecord(id)
	if err == nil {
		t.Fatal("expected incomplete deployment record error")
	}
	stateErr, ok := DeploymentRecordState(err)
	if !ok {
		t.Fatalf("expected DeploymentRecordStateError, got %T: %v", err, err)
	}
	if stateErr.Kind != "incomplete" ||
		stateErr.Identity.Target != id.Target ||
		stateErr.Identity.DeploymentID != id.DeploymentID {
		t.Fatalf("unexpected state error: %#v", stateErr)
	}
	if stateErr.Identity.ApplicationID != "" || stateErr.Identity.Application != "" || stateErr.Identity.Environment != "" {
		t.Fatalf("missing deployment record must not invent semantic identity: %#v", stateErr.Identity)
	}
}

func TestSameApplicationEnvironmentAcrossTargets(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, target := range []string{"docker-dev", "podman-dev"} {
		if err := SaveDeploymentRecord(DeploymentRecord{
			Identity: testDeploymentIdentity(t, target, "demo", "dev"),
			Applied:  AppliedDeployment{RuntimeProvider: target},
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

func TestMultipleEnvironmentsWithinOneTarget(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, environment := range []string{"dev", "test"} {
		if err := SaveDeploymentRecord(DeploymentRecord{
			Identity: testDeploymentIdentity(t, "docker-dev", "demo", environment),
			Applied:  AppliedDeployment{RuntimeProvider: "docker"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListDeployments("docker-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("deployments = %d, want 2", len(got))
	}
	seen := map[string]bool{}
	for _, record := range got {
		seen[record.Identity.Environment] = true
	}
	if !seen["dev"] || !seen["test"] {
		t.Fatalf("environments missing from target registry: %#v", seen)
	}
}

func TestDeploymentRootUsesStableDeploymentIDNotReadableNames(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id := testDeploymentIdentity(t, "docker-dev", "demo", "dev")
	before, err := DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	renamed := id
	renamed.Application = "renamed-demo"
	after, err := DeploymentRoot(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("deployment root changed after readable rename: before=%q after=%q", before, after)
	}
	if filepath.Base(before) != id.DeploymentID {
		t.Fatalf("deployment state root is not keyed by deployment ID: %q", before)
	}
}

func TestFindDeploymentUsesApplicationIDAcrossReadableRename(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id := testDeploymentIdentity(t, "docker-dev", "demo", "dev")
	if err := SaveDeploymentRecord(DeploymentRecord{
		Identity: id,
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}); err != nil {
		t.Fatal(err)
	}

	record, found, err := FindDeployment("docker-dev", testApplicationID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !found || record.Identity.DeploymentID != id.DeploymentID {
		t.Fatalf("stable deployment lookup failed: found=%t record=%#v", found, record)
	}

	record.Identity.Application = "renamed-demo"
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	again, found, err := FindDeployment("docker-dev", testApplicationID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !found || again.Identity.DeploymentID != id.DeploymentID || again.Identity.Application != "renamed-demo" {
		t.Fatalf("rename did not preserve stable deployment identity: %#v", again.Identity)
	}
}

func TestFindDeploymentRejectsAmbiguousStableApplicationOwnership(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	first := testDeploymentIdentity(t, "docker-dev", "alpha", "dev")
	second := testDeploymentIdentity(t, "docker-dev", "renamed-alpha", "dev")
	if first.DeploymentID == second.DeploymentID {
		t.Fatal("expected unique deployment IDs")
	}
	for _, id := range []DeploymentIdentity{first, second} {
		if err := SaveDeploymentRecord(DeploymentRecord{
			Identity: id,
			Applied:  AppliedDeployment{RuntimeProvider: "docker"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := FindDeployment("docker-dev", testApplicationID, "dev"); err == nil {
		t.Fatal("ambiguous stable application ownership was accepted")
	}
}

func TestLoadDeploymentRecordAllowsReadableRenameWithStableIdentity(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id := testDeploymentIdentity(t, "docker-dev", "alpha", "dev")
	record := DeploymentRecord{
		Identity: id,
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}

	requested := id
	requested.Application = "renamed-alpha"
	got, err := LoadDeploymentRecord(requested)
	if err != nil {
		t.Fatalf("stable identity lookup rejected readable rename: %v", err)
	}
	if got.Identity.DeploymentID != id.DeploymentID || got.Identity.ApplicationID != id.ApplicationID {
		t.Fatalf("stable identity changed across readable rename: %#v", got.Identity)
	}
}


func TestDeleteDeploymentRecordRemovesDeploymentRoot(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	id := testDeploymentIdentity(t, "docker-dev", "demo", "dev")
	record := DeploymentRecord{
		Identity: id,
		Applied:  AppliedDeployment{RuntimeProvider: "docker"},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	root, err := DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatalf("deployment record was not materialized: %v", err)
	}
	if err := DeleteDeploymentRecord(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("deployment root remains after delete: %s err=%v", root, err)
	}
	if err := DeleteDeploymentRecord(id); err != nil {
		t.Fatalf("repeated delete must be idempotent: %v", err)
	}
}
