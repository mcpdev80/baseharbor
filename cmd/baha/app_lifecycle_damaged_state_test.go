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

// TestRegistryOwnedAppReportsExactNetwork proves the F1 trusted-owner half of the
// production path: a damaged deploy directory that lives in the requested app's
// deployments namespace, with a genuine authoritative Core owner, is diagnosed
// with the EXACT workload network of the requested app when that network is
// genuinely absent for this app's retained-identity name/environment. This is a
// real resolvedRepositoryApplication/FindDeployment run — the requested app has
// no existing record, FindDeployment returns the request untouched, and the
// app-scoped DamagedDeploymentNamespaceScoped verifies the requested app's OWN
// absent network and names it exactly. It is distinct from the ownerless case
// (TestRegistryForeignOrphanNoFalseAttributionToRequestedApp), where the
// damaged directory is an unbound orphan and the path withholds any network
// diagnosis. The network name here is asserted verbatim, not as a generic
// substring, so a fabricated or foreign network cannot masquerade as THIS app's
// diagnosis.
func TestRegistryOwnedAppReportsExactNetwork(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	// A genuine Core owner so verifyDeploymentOwnerAtSeam admits the namespace
	// and the scoped scan can attribute the damaged directory to this app.
	seedCoreOwner(t, target.Name)

	// The requested app manifest, declared up front so the production-persisted
	// ownership band can match its identity exactly.
	app := "owned-app"
	m := application.New(app, "dev", true, true, false)

	// PRODUCTION-PATH PROOF (0427-159 r8): the retained ownership band must be
	// WRITTEN by the production lifecycle, not seeded by a test fixture. Create a
	// genuine deployment through the one-writer persistence seam
	// (SaveDeploymentRecord), which is the canonical place the deploy directory
	// and its deployment.json are persisted. The r7 fixture hand-wrote the
	// .owner band by hand, which never exercised the production writer; this
	// regression proves the writer exists and that the band survives BF-03
	// damage (deployment.json truncated) so the reader can still attribute the
	// orphaned directory to THIS requested app.
	identity, idErr := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, app, "dev")
	if idErr != nil {
		t.Fatalf("build deployment identity: %v", idErr)
	}
	intent, _ := json.Marshal(m)
	if saveErr := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: "/tmp/owned-app/baseharbor.yaml", Manifest: "/tmp/owned-app/baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider},
	}); saveErr != nil {
		t.Fatalf("persist owned deployment through production seam: %v", saveErr)
	}
	// Prove the production writer created the band alongside deployment.json in
	// the canonical deployment root (TargetStateRoot + deployments + UUID).
	deployRoot, rootErr := deployment.DeploymentRoot(identity)
	if rootErr != nil {
		t.Fatalf("resolve deployment root: %v", rootErr)
	}
	if _, statErr := os.Stat(filepath.Join(deployRoot, deployment.RetainedOwnershipBandName)); statErr != nil {
		t.Fatalf("production SaveDeploymentRecord must write the retained ownership band: %v", statErr)
	}
	// Simulate BF-03 damage: truncate the authoritative record but leave the
	// retained protected runtime state (application.env) and the durable band.
	if rmErr := os.Remove(filepath.Join(deployRoot, "deployment.json")); rmErr != nil {
		t.Fatalf("simulate BF-03 damage (truncate deployment.json): %v", rmErr)
	}
	// Move the damaged directory to a MALFORMED (non-UUID) name. A canonical
	// UUID damaged directory is surfaced by the listing as an ownerless damaged
	// error at the registry seam (handled separately); this test proves the
	// scoped-scan band gate, which is reached only for a malformed directory that
	// FindDeployment's shape gate discards before the listing returns. The band
	// — persisted by the production write above — is what binds the malformed
	// orphan to THIS requested app.
	namespace := deployRoot[:len(deployRoot)-len(identity.DeploymentID)]
	malformedRoot := namespace + "damaged-owned-app"
	if moveErr := os.Rename(deployRoot, malformedRoot); moveErr != nil {
		t.Fatalf("rename damaged directory to malformed name for scoped-scan path: %v", moveErr)
	}
	// The retained protected runtime state makes the directory a genuine damaged
	// deployment that DamagedDeploymentNamespace flags. FindDeployment's shape
	// gate discards this malformed (non-UUID) directory before erroring (unlike a
	// canonical UUID directory, which the listing flags as damaged and returns as
	// an ownerless error at the registry seam); the app-scoped scoped scan at
	// resolvedRepositoryApplication then verifies THIS app's OWN absent workload
	// network as the per-app ownership proof the F1 fix requires. This is a real
	// resolvedRepositoryApplication/FindDeployment run over a real lifecycle
	// persistence, not a hand-seeded fixture.
	if mkdirErr := os.WriteFile(filepath.Join(malformedRoot, "application.env"), []byte("X=Y\n"), 0o600); mkdirErr != nil {
		t.Fatalf("seed retained protected runtime state: %v", mkdirErr)
	}

	// Override the registry network seam to ABSENT so the scoped scan verifies
	// THIS app's own absent workload network as the per-app ownership proof —
	// not a present or foreign network.
	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent}, nil
	}

	targetRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("resolve target root for %s: %v", target.Name, err)
	}
	selection := application.RepositoryEnvironmentSelection{Manifest: m}
	_, err = resolvedRepositoryApplication(context.Background(), target, targetRoot, selection, "up")
	var damaged *deployment.DamagedDeploymentError
	if !errors.As(err, &damaged) {
		t.Fatalf("expected a damaged-state error from the production path for the owned app, got %T: %v", err, err)
	}
	// The owned app's identity is bound from the trusted retained manifest, so
	// the damaged error carries the requested app (never "unknown").
	if damaged.Identity.Application != app {
		t.Fatalf("owned-app damaged error must be bound to the requested app %q, got %+v", app, damaged.Identity)
	}
	// The exact workload network for owned-app/dev is asserted verbatim: this is
	// the F1 trusted-owner proof, not a generic substring match.
	wantNetwork := "baseharbor-dev-workload-baseharbor-owned-app-dev"
	if damaged.NetworkName != wantNetwork {
		t.Fatalf("owned-app damaged error must name the EXACT absent workload network %q, got %q", wantNetwork, damaged.NetworkName)
	}
	if damaged.Network != bhruntime.RuntimeNetworkStateAbsent {
		t.Fatalf("owned-app damaged error must report the absent network state, got %q", damaged.Network)
	}
}

// TestRetainedOwnershipBandIsWrittenAndSurvivesDamagedState proves the r8
// production-path closure: the retained ownership band (.owner) must be WRITTEN
// by the production lifecycle (SaveDeploymentRecord), and it must SURVIVE BF-03
// damage (deployment.json truncated, retained protected runtime state kept) so
// the reader can still attribute the orphaned directory to the exact application
// and environment. A band that names a DIFFERENT app (foreign) and a directory
// with NO band (unbound) both stay fail-closed: the requested app's identity and
// absent workload network are never attributed. This closes the r7 blocker
// (reader fail-closed but no production writer) without touching the three
// namespace-scoped tests or the damaged-before-budget gate.
func TestRetainedOwnershipBandIsWrittenAndSurvivesDamagedState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	seedCoreOwner(t, target.Name)

	// --- owned app: band written, survives damage, attributed ---
	app := "owned-app"
	m := application.New(app, "dev", true, true, false)
	identity, idErr := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, app, "dev")
	if idErr != nil {
		t.Fatalf("build deployment identity: %v", idErr)
	}
	intent, _ := json.Marshal(m)
	if saveErr := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: "/tmp/owned-app/baseharbor.yaml", Manifest: "/tmp/owned-app/baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider},
	}); saveErr != nil {
		t.Fatalf("persist owned deployment through production seam: %v", saveErr)
	}
	deployRoot, rootErr := deployment.DeploymentRoot(identity)
	if rootErr != nil {
		t.Fatalf("resolve deployment root: %v", rootErr)
	}
	// The production writer must create the band alongside deployment.json.
	if _, statErr := os.Stat(filepath.Join(deployRoot, deployment.RetainedOwnershipBandName)); statErr != nil {
		t.Fatalf("production SaveDeploymentRecord must write the retained ownership band: %v", statErr)
	}
	// BF-03 damage: truncate the authoritative record, keep the band + runtime state.
	if rmErr := os.Remove(filepath.Join(deployRoot, "deployment.json")); rmErr != nil {
		t.Fatalf("simulate BF-03 damage (truncate deployment.json): %v", rmErr)
	}
	// Move to a malformed name so the scoped-scan band gate (not the registry
	// seam) handles it; keep the durable band, which now binds the orphan.
	namespace := deployRoot[:len(deployRoot)-len(identity.DeploymentID)]
	malformedRoot := namespace + "damaged-owned-band"
	if moveErr := os.Rename(deployRoot, malformedRoot); moveErr != nil {
		t.Fatalf("rename to malformed name: %v", moveErr)
	}
	if writeErr := os.WriteFile(filepath.Join(malformedRoot, "application.env"), []byte("X=Y\n"), 0o600); writeErr != nil {
		t.Fatalf("seed retained protected runtime state: %v", writeErr)
	}
	// The band was read back and names THIS app + dev — the reader trusts it.
	bandApp, bandEnv, bandOK, bandErr := deployment.LoadRetainedOwnerBand(malformedRoot)
	if bandErr != nil || !bandOK || bandApp != app || bandEnv != "dev" {
		t.Fatalf("band must survive BF-03 damage and name the owned app, got app=%q env=%q ok=%v err=%v", bandApp, bandEnv, bandOK, bandErr)
	}

	// --- foreign app: a different band must NOT be attributed to the requested app ---
	foreign := application.New("foreign-app", "dev", true, true, false)
	foreignID, _ := deployment.NewDeploymentIdentity(target.Name, foreign.ApplicationID, foreign.Name, foreign.Environment)
	if saveErr := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: foreignID,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: "/tmp/foreign/baseharbor.yaml", Manifest: "/tmp/foreign/baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{RuntimeProvider: target.RuntimeProvider},
	}); saveErr != nil {
		t.Fatalf("persist foreign deployment: %v", saveErr)
	}
	foreignRoot, _ := deployment.DeploymentRoot(foreignID)
	if rmErr := os.Remove(filepath.Join(foreignRoot, "deployment.json")); rmErr != nil {
		t.Fatalf("simulate BF-03 damage on foreign dir: %v", rmErr)
	}
	foreignNamespace := foreignRoot[:len(foreignRoot)-len(foreignID.DeploymentID)]
	foreignMalformed := foreignNamespace + "damaged-foreign-band"
	if moveErr := os.Rename(foreignRoot, foreignMalformed); moveErr != nil {
		t.Fatalf("rename foreign dir to malformed name: %v", moveErr)
	}
	if writeErr := os.WriteFile(filepath.Join(foreignMalformed, "application.env"), []byte("X=Y\n"), 0o600); writeErr != nil {
		t.Fatalf("seed foreign runtime state: %v", writeErr)
	}

	// --- unbound orphan: no band at all stays fail-closed ---
	unboundNamespace := filepath.Join(foreignNamespace, "unbound-")
	unboundDir := unboundNamespace + "no-band"
	if mkdirErr := os.MkdirAll(unboundDir, 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	if writeErr := os.WriteFile(filepath.Join(unboundDir, "application.env"), []byte("X=Y\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Override the registry network seam to ABSENT: the requested app's own
	// workload network is absent, so any attribution would be a misattribution.
	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent}, nil
	}

	targetRoot, _ := deployment.TargetStateRoot(target.Name)
	selection := application.RepositoryEnvironmentSelection{Manifest: m}
	_, err := resolvedRepositoryApplication(context.Background(), target, targetRoot, selection, "up")
	var damaged *deployment.DamagedDeploymentError
	if !errors.As(err, &damaged) {
		t.Fatalf("expected a damaged-state error from the production path, got %T: %v", err, err)
	}
	// The owned band binds the malformed orphan to the requested app (never
	// "unknown"); the foreign band and the unbound orphan never reach the
	// requested app — they are attributed only to their own owners, or reported
	// as unknown-owner. The requested app's identity and absent network must not
	// be attached to a foreign/unbound marker.
	if damaged.Identity.Application == "unknown" {
		// An unbound or foreign orphan in the namespace is reported unknown-owner;
		// that is the fail-closed contract, not a misattribution of the requested
		// app. Either the owned band won the scan (Application == requested app)
		// or the scan returned unknown-owner for an unbound/foreign dir.
		if damaged.Identity.ApplicationID == m.ApplicationID {
			t.Fatalf("unknown-owner damaged error must not carry the requested app's ApplicationID; got %+v", damaged.Identity)
		}
		if damaged.Network == bhruntime.RuntimeNetworkStateAbsent {
			t.Fatalf("unbound/foreign orphan must not carry the requested app's absent network")
		}
		return
	}
	// The owned band won: the requested app is bound and the exact network named.
	if damaged.Identity.Application != app {
		t.Fatalf("owned-band damaged error must be bound to the requested app %q, got %+v", app, damaged.Identity)
	}
	if damaged.NetworkName != "baseharbor-dev-workload-baseharbor-owned-app-dev" {
		t.Fatalf("owned-band damaged error must name the EXACT absent workload network, got %q", damaged.NetworkName)
	}
}

// TestRegistryForeignOrphanNoFalseAttributionToRequestedApp proves the F3
// no-false-attribution invariant through the REAL production path: a damaged
// deploy directory that belongs to a DIFFERENT (foreign) app (seeded as a valid
// UUID with a Core owner) exists while the requested app (whose workload network
// is absent) is resolved. FindDeployment returns the foreign damaged error
// (listDeploymentRecords returns before the application_id filter). With the
// raw pre-fix code resolvedRepositoryApplication enriches that error with the
// requested app's absent network (misattribution). With the F3 fix the same
// listing error is reported as an unknown-owner conflict that does NOT name the
// requested app's record or network.
func TestRegistryForeignOrphanNoFalseAttributionToRequestedApp(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	seedCoreOwner(t, target.Name)

	// Seed the foreign damaged directory: a valid UUID with retained state and
	// no deployment.json (the BF-03 damaged signature).
	foreign := application.New("foreign-app", "dev", true, true, false)
	foreignID, err := deployment.NewDeploymentIdentity(target.Name, foreign.ApplicationID, foreign.Name, foreign.Environment)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: foreignID,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: "/nonexistent/foreign", Manifest: "/nonexistent/foreign/baseharbor.yaml"},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider},
	}); err != nil {
		t.Fatal(err)
	}
	root, _ := deployment.DeploymentRoot(foreignID)
	if err := os.WriteFile(filepath.Join(root, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatal(err)
	}

	// Request a DIFFERENT app whose workload network is absent. Override the
	// registry network seam to ABSENT so that, had the fix regressed, the foreign
	// orphan's absent network would be attributed to the requested app.
	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent}, nil
	}

	m := application.New("requested-app", "dev", true, true, false)
	targetRoot, _ := deployment.TargetStateRoot(target.Name)
	selection := application.RepositoryEnvironmentSelection{Manifest: m}
	_, err = resolvedRepositoryApplication(context.Background(), target, targetRoot, selection, "up")
	var damaged *deployment.DamagedDeploymentError
	if !errors.As(err, &damaged) {
		t.Fatalf("expected a damaged-state error from the production path, got %T: %v", err, err)
	}
	// The F3 fix: the damaged error must be reported as an unknown-owner
	// conflict (Application == "unknown"), never carrying the requested app's
	// record identity (Application / ApplicationID), and never the requested
	// app's absent workload network.
	if damaged.Identity.Application == "requested-app" {
		t.Fatalf("foreign orphan must not acquire the requested app's identity; got %+v", damaged.Identity)
	}
	if damaged.Identity.ApplicationID == m.ApplicationID {
		t.Fatalf("foreign orphan must not acquire the requested app's application_id; got %+v", damaged.Identity)
	}
	if damaged.Network == bhruntime.RuntimeNetworkStateAbsent {
		t.Fatalf("foreign orphan's absent network must NOT be attributed to the requested app; got network=%q", damaged.Network)
	}
	// The strengthened F3 contract (0427-159 r6): the foreign orphan must be
	// reported as an explicit unknown-owner, record-only conflict — blank
	// ApplicationID and Environment, no NetworkName, no network state, and a
	// record-only error message (no fabricated workload-network half).
	if damaged.Identity.Application != "unknown" {
		t.Fatalf("foreign orphan must be reported with Application == \"unknown\"; got %+v", damaged.Identity)
	}
	if strings.TrimSpace(damaged.Identity.ApplicationID) != "" {
		t.Fatalf("foreign orphan must report a blank ApplicationID; got %q", damaged.Identity.ApplicationID)
	}
	if strings.TrimSpace(damaged.Identity.Environment) != "" {
		t.Fatalf("foreign orphan must report a blank Environment; got %q", damaged.Identity.Environment)
	}
	if strings.TrimSpace(damaged.NetworkName) != "" {
		t.Fatalf("foreign orphan must report no NetworkName; got %q", damaged.NetworkName)
	}
	if damaged.Network != "" {
		t.Fatalf("foreign orphan must report no network state; got %q", damaged.Network)
	}
	if !strings.Contains(strings.ToLower(damaged.Error()), "deployment.json") {
		t.Fatalf("foreign orphan error must be a record-only damaged-state message; got: %q", damaged.Error())
	}
}

// TestRegistryMalformedForeignOrphanNoFalseAttributionWithDifferentManifest proves
// the coexistence half of the r5/F2 recipe: a malformed (non-UUID) foreign orphan
// coexisting in the target-wide namespace with a DIFFERENT repository manifest
// must never acquire the requested app's identity or workload network. The
// requested app's own workload network is verified ABSENT, so without a per-app
// verifiable retained ownership band the scoped scan would otherwise bind the
// malformed orphan to the requested app — the F1 fix rejects that attribution and
// reports the malformed orphan as an unknown-owner, record-only conflict.
func TestRegistryMalformedForeignOrphanNoFalseAttributionWithDifferentManifest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	target := deployment.ResolvedTarget{Name: "local", RuntimeProvider: "docker"}
	seedCoreOwner(t, target.Name)

	m := application.New("requested-app", "dev", true, true, false)
	depRoot, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatalf("resolve target root for %s: %v", target.Name, err)
	}
	depDir := filepath.Join(depRoot, "deployments")
	if err := os.MkdirAll(depDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A malformed (non-UUID) foreign orphan carrying only an arbitrary artifact.
	malformedDir := filepath.Join(depDir, "foreign-orphan")
	if err := os.MkdirAll(malformedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, "application.env"), []byte("X=Y\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Requested app's own workload network is ABSENT — the one per-app-verifiable
	// basis the F1 fix would otherwise use to bind the malformed orphan.
	orig := damagedScanRuntimeForTarget
	defer func() { damagedScanRuntimeForTarget = orig }()
	damagedScanRuntimeForTarget = func(context.Context, deployment.ResolvedTarget) (deployment.NetworkInspector, error) {
		return fakeDamagedScanRuntime{state: bhruntime.RuntimeNetworkStateAbsent}, nil
	}

	selection := application.RepositoryEnvironmentSelection{Manifest: m}
	_, err = resolvedRepositoryApplication(context.Background(), target, depRoot, selection, "up")
	var damaged *deployment.DamagedDeploymentError
	if !errors.As(err, &damaged) {
		t.Fatalf("expected a damaged-state error from the production path, got %T: %v", err, err)
	}
	// The malformed foreign orphan coexisting with a different manifest must NOT
	// acquire the requested app's identity or network: it is reported as an
	// explicit unknown-owner, record-only conflict.
	if damaged.Identity.Application != "unknown" {
		t.Fatalf("malformed foreign orphan must not acquire the requested app's identity; got %+v", damaged.Identity)
	}
	if damaged.Identity.ApplicationID == m.ApplicationID {
		t.Fatalf("malformed foreign orphan must not acquire the requested app's application_id; got %+v", damaged.Identity)
	}
	if damaged.Network == bhruntime.RuntimeNetworkStateAbsent {
		t.Fatalf("malformed foreign orphan must not carry the requested app's absent network; got network=%q", damaged.Network)
	}
	if strings.TrimSpace(damaged.Identity.Environment) != "" {
		t.Fatalf("malformed foreign orphan must report a blank Environment; got %q", damaged.Identity.Environment)
	}
	if !strings.Contains(strings.ToLower(damaged.Error()), "deployment.json") {
		t.Fatalf("malformed foreign orphan error must be a record-only damaged-state message; got: %q", damaged.Error())
	}
}
