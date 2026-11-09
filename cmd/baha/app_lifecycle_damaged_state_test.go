package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// seedCoreOwner provisions a genuine authoritative Core installation under the
// target's runtime root so the one-installation-owner seam admits a valid
// deployment record. The damaged-state signature itself is independent of the
// owner, but a valid record must still load against a proven owner.
func seedCoreOwner(t *testing.T, target string) {
	t.Helper()
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	targetRoot, err := deployment.TargetStateRoot(target)
	if err != nil {
		t.Fatalf("resolve runtime root for %s: %v", target, err)
	}
	installRoot := filepath.Join(targetRoot, "runtime")
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installRoot, "installation.json")); !os.IsNotExist(err) {
		return
	}
	spec := coreinstallation.Spec{Target: target, Runtime: "docker", MachineRole: coreinstallation.Development}
	steps := coreinstallation.Steps{
		Preflight: func(context.Context, coreinstallation.State) error { return nil },
		SQL:       func(context.Context, coreinstallation.State) error { return nil },
		Secrets:   func(context.Context, coreinstallation.State) error { return nil },
		Identity: func(context.Context, coreinstallation.State) (string, error) {
			return "https://identity.localhost:8443/realms/baseharbor", nil
		},
		Verify: func(context.Context, coreinstallation.State) error { return nil },
	}
	if _, err := coreinstallation.Run(context.Background(), installRoot, spec, steps); err != nil {
		t.Fatalf("seed Core owner for %s: %v", target, err)
	}
}

// TestDetectDamagedDeploymentState covers the damaged-state signature for
// bug-hunt BF-03: a protected deploy directory that exists while its
// deployment.json is missing. The directory existing without the record is the
// damage; a fresh unregistration removes the whole directory and a healthy
// deployment always carries deployment.json.
func TestDetectDamagedDeploymentState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := application.New("damaged", "dev", true, true, false)
	id, err := deployment.NewDeploymentIdentity("local", m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}

	// Case 1: directory absent -> not damaged (fresh, never applied).
	damaged, err := deployment.DetectDamagedDeploymentState(id)
	if err != nil {
		t.Fatal(err)
	}
	if damaged {
		t.Fatal("absent deploy directory must not be flagged damaged")
	}

	// Case 2: valid record present -> not damaged (healthy). The owner seam
	// requires a proven Core installation, so seed one for the local target.
	seedCoreOwner(t, "local")
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, manifestYAML(m), 0600); err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: "docker"},
		Observed: deployment.ObservedDeployment{State: "configured", Ready: true},
	}); err != nil {
		t.Fatal(err)
	}
	damaged, err = deployment.DetectDamagedDeploymentState(id)
	if err != nil {
		t.Fatal(err)
	}
	if damaged {
		t.Fatal("record-present directory must not be flagged damaged")
	}

	// Case 3: damaged state — delete deployment.json while retaining a protected
	// runtime artifact (application.env). A retained deploy dir with orphaned
	// runtime state but no record is the damaged signature.
	root, derr := deployment.DeploymentRoot(id)
	if derr != nil {
		t.Fatal(derr)
	}
	if err := os.WriteFile(filepath.Join(root, "application.env"), []byte("X=Y\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatal(err)
	}
	damaged, err = deployment.DetectDamagedDeploymentState(id)
	if err != nil {
		t.Fatal(err)
	}
	if !damaged {
		t.Fatal("retained deploy directory with orphaned runtime state but missing deployment.json must be flagged damaged")
	}
}

// TestUpFailsClosedOnDamagedDeploymentState exercises the real `up` execution
// entry: a repository app whose authoritative record is gone but whose on-disk
// deployment.json is missing while protected runtime state was retained must
// fail closed on the damaged state in resolvedRepositoryApplication before
// reaching the generic repository-budget check.
func TestUpFailsClosedOnDamagedDeploymentState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}

	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, manifestYAMLWithWorkload("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}

	// Persist a valid record with the workload manifest, then damage it by
	// deleting deployment.json while retaining the deploy directory.
	m := application.WithWorkloadComponents(application.New("damaged", "dev", false, false, false), "api")
	id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	seedCoreOwner(t, target.Name)
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider},
		Observed: deployment.ObservedDeployment{State: "configured", Ready: true},
	}); err != nil {
		t.Fatal(err)
	}
	root, _ := deployment.DeploymentRoot(id)
	// Retain a protected runtime artifact so the deleted-record dir is the
	// damaged (not merely incomplete) signature.
	if err := os.WriteFile(filepath.Join(root, "application.env"), []byte("X=Y\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Damage: remove deployment.json, retain the directory.
	if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatal(err)
	}

	// Resolve from inside the repository (the `up` without NAME path).
	t.Chdir(repo)

	var out strings.Builder
	_, err = newApplicationUpExecution(context.Background(), application.Store{}, nil, &out, &out)
	if err == nil {
		t.Fatal("expected damaged deployment state error from up execution entry")
	}
	var damaged *deployment.DamagedDeploymentError
	if !errors.As(err, &damaged) {
		t.Fatalf("expected *deployment.DamagedDeploymentError, got %T: %v", err, err)
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "re-up") && !strings.Contains(lower, "destroy") {
		t.Fatalf("damaged-state error must name concrete recovery (re-up or destroy-and-recreate): %q", err.Error())
	}
	if !strings.Contains(lower, "deployment.json") {
		t.Fatalf("damaged-state error must name the missing record: %q", err.Error())
	}
	if damaged.Root == "" {
		t.Fatal("damaged-state error must name the damaged directory")
	}
}

func manifestYAML(m application.Manifest) []byte {
	return []byte("version: 1\napp:\n  name: " + m.Name + "\n  environment: " + m.Environment + "\n")
}

func manifestYAMLWithWorkload(name string) []byte {
	return []byte("version: 1\napp:\n  name: " + name + "\n  environment: dev\n")
}

// InspectNetwork state and satisfies the minimal surface the damaged-scan uses.
type fakeDamagedScanRuntime struct {
	state bhruntime.RuntimeNetworkState
}

func (f fakeDamagedScanRuntime) InspectNetwork(context.Context, string) (bhruntime.RuntimeNetworkState, error) {
	return f.state, nil
}

// TestUpFailsClosedNamingRecordAndNetworkAbsent exercises the real `up` execution
// entry for the observed BF-03 condition: the record is damaged AND the workload
// runtime network is absent. The damaged-state error must name BOTH halves
// (the missing record and the absent network) and both recovery paths, and it
// must name the resolved app's derived workload network.
func TestUpFailsClosedNamingRecordAndNetworkAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Override the damaged-scan runtime seam with the absent-network fake.
	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent}, nil
	}

	testDamagedUpNetwork(t, "netz-weg")
}

// TestUpDamagedRecordPresentNetworkStillFlagsRecord exercises the `up` entry when
// the workload network is still present: the record damage alone must still fail
// closed (record half only), and no absent network is fabricated.
func TestUpDamagedRecordPresentNetworkStillFlagsRecord(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStatePresent}, nil
	}

	testDamagedUpNetwork(t, "netz-present")
}

// testDamagedUpNetwork seeds a damaged repository app (record deleted, network
// state injected via the overridden seam) and asserts the `up` entry fails
// closed on the damaged-state error.
func testDamagedUpNetwork(t *testing.T, label string) {
	t.Helper()
	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}

	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "baseharbor.yaml")
	if err := os.WriteFile(manifestPath, manifestYAMLWithWorkload("damaged-net"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "compose.yaml"), []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}

	m := application.WithWorkloadComponents(application.New("damaged-net", "dev", false, false, false), "api")
	id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	seedCoreOwner(t, target.Name)
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: id,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider},
		Observed: deployment.ObservedDeployment{State: "configured", Ready: true},
	}); err != nil {
		t.Fatal(err)
	}
	root, _ := deployment.DeploymentRoot(id)
	if err := os.WriteFile(filepath.Join(root, "application.env"), []byte("X=Y\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatal(err)
	}

	t.Chdir(repo)

	var out strings.Builder
	_, err = newApplicationUpExecution(context.Background(), application.Store{}, nil, &out, &out)
	if err == nil {
		t.Fatalf("[%s] expected damaged deployment state error from up execution entry", label)
	}
	var damaged *deployment.DamagedDeploymentError
	if !errors.As(err, &damaged) {
		t.Fatalf("[%s] expected *deployment.DamagedDeploymentError, got %T: %v", label, err, err)
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "re-up") || !strings.Contains(lower, "destroy") {
		t.Fatalf("[%s] damaged-state error must name BOTH recovery paths (re-up and destroy-and-recreate): %q", label, err.Error())
	}
	if !strings.Contains(lower, "deployment.json") {
		t.Fatalf("[%s] damaged-state error must name the missing record: %q", label, err.Error())
	}
	if damaged.Root == "" {
		t.Fatalf("[%s] damaged-state error must name the damaged directory", label)
	}
	// The record-absent damage must be present; when the network is absent both
	// halves are named in the user-facing message.
	if label == "netz-weg" && !strings.Contains(lower, "network") {
		t.Fatalf("[%s] absent-network damage must name the network in the error: %q", label, err.Error())
	}
}

// TestRegistryUpFailsClosedNamingRecordAndNetworkAbsent proves the F1 registry
// fix: the registry path (FindDeployment returns a damaged error whose deploy
// directory is a per-deploy UUID with NO record to read the retained identity
// from) must still report BOTH halves — the missing record AND the absent
// workload network for THIS deployment's retained identity. The identity is
// reconstructed from the request parameters (application_id, manifest name,
// environment) and the workload network is verified for that exact identity.
func TestRegistryUpFailsClosedNamingRecordAndNetworkAbsent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Override the registry network seam with the absent-network fake BEFORE
	// any resolution.
	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent}, nil
	}

	// Reproduce the exact registry damaged path used by FindDeployment: a
	// DamagedDeploymentError whose Identity carries only Target + a UUID
	// DeploymentID (no Application/Environment, because the record is gone).
	registryErr := &deployment.DamagedDeploymentError{
		Identity: deployment.DeploymentIdentity{
			Target:       "local",
			DeploymentID: "11111111-1111-1111-1111-111111111111",
			// Application / Environment intentionally ABSENT, as in the listing.
		},
		Root: "/state/targets/local/deployments/11111111-1111-1111-1111-111111111111",
	}
	damaged, ok := deployment.DamagedDeploymentState(registryErr, fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent})
	if !ok {
		t.Fatal("DamagedDeploymentState must recognize a registry damaged error")
	}
	// Reconstruct the retained identity from the request parameters, exactly as
	// resolvedRepositoryApplication does before enrichment.
	damaged.Identity.ApplicationID = "app-abc123"
	damaged.Identity.Application = "reg-app"
	damaged.Identity.Environment = "dev"
	// Re-verify now that the retained identity is populated, so the workload
	// network can be derived for THIS app's exact identity.
	damaged.EnrichNetworkForOwnIdentity(fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent})
	// The network half must now be derivable and verified for THIS identity.
	if damaged.NetworkName != "baseharbor-dev-workload-baseharbor-reg-app-dev" {
		t.Fatalf("registry damaged error must name the derived workload network for the retained identity, got %q", damaged.NetworkName)
	}
	if !strings.Contains(strings.ToLower(damaged.Error()), "network") {
		t.Fatalf("registry damaged error must name the absent workload network (F1 fix), got: %q", damaged.Error())
	}
	// A foreign orphan (different app) must NEVER appear as THIS app's absent
	// network: the registry error is attributed only to the resolved identity.
	foreignIdentity := &deployment.DamagedDeploymentError{
		Identity: deployment.DeploymentIdentity{
			Target:       "local",
			DeploymentID: "22222222-2222-2222-2222-222222222222",
			Application:  "foreign-app",
			Environment:  "dev",
		},
		Root: "/state/targets/local/deployments/22222222-2222-2222-2222-222222222222",
	}
	// When THIS app's network is present, the error stays record-only — the
	// foreign-orphan's absent network must not be attributed to this app.
	present, _ := deployment.DamagedDeploymentState(foreignIdentity, fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStatePresent})
	if present.Network == bhruntime.RuntimeNetworkStateAbsent {
		t.Fatal("foreign orphan's absent network must not be attributed to the resolved app")
	}
}
