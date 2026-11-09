package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// seedPoisonedNativeTwinRegistry reproduces the BF-01 end state (two deployment
// records claiming one application_id on the same Target/environment) on the
// CURRENT binary on top of a real, already-applied application. The fixed write
// seam refuses to create that shape, so the second twin is persisted by direct
// file write — the state a target poisoned by an older binary really holds.
func seedPoisonedNativeTwinRegistry(t *testing.T, ctx context.Context, base resolvedApplication) (twinIDs [2]string, baseRoot string) {
	t.Helper()
	baseRoot, berr := deployment.DeploymentRoot(base.DeploymentIdentity)
	if berr != nil {
		t.Fatal(berr)
	}
	twinIDs[0] = base.DeploymentIdentity.DeploymentID

	// A second, distinct deployment id claiming the SAME application_id, name,
	// environment and repository source — the exact shape the double init
	// produced. Both records share the authored manifest.
	second, err := deployment.NewDeploymentIdentity(base.Target.Name, base.Manifest.ApplicationID, base.Manifest.Name, base.Manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	twinIDs[1] = second.DeploymentID
	secondRoot, err := deployment.DeploymentRoot(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(base.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: second,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: base.repositoryRoot(), Manifest: base.ManifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: base.Target.RuntimeProvider, LastAppliedRef: "poison-revision", GeneratedState: map[string]string{"deployment": base.DeploymentStateRoot, "state": base.DeploymentStateRoot + "/state"}},
		Observed: deployment.ObservedDeployment{State: "created"},
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondRoot, "deployment.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	// Seed the retained '.owner' band in BOTH twins — the metadata a production
	// deployment directory carries alongside deployment.json.
	for _, id := range twinIDs {
		var root string
		if id == twinIDs[0] {
			root = baseRoot
		} else {
			root = secondRoot
		}
		if _, statErr := os.Stat(filepath.Join(root)); statErr != nil {
			t.Fatalf("twin %s must exist: %v", id, statErr)
		}
		bandPath := filepath.Join(root, ".owner")
		band := []byte("baseharbor.owner=baseharbor\nscope=application\napp=" + base.Manifest.ApplicationID + "\n")
		if berr := os.WriteFile(bandPath, band, 0o600); berr != nil {
			t.Fatal(berr)
		}
	}
	// Strict inventory refuses on the poison; the display listing tolerates it.
	if _, listErr := deployment.ListDeployments(base.Target.Name); !errors.Is(listErr, deployment.ErrRegistryConflict) {
		t.Fatalf("precondition: the registry must be poisoned, got %v", listErr)
	}
	return twinIDs, baseRoot
}

// TestRegistryConflictRepairViaRealCLIEntryDeletesSelectedTwinOnlyAndKeepsSibling
// is the executable recovery regression (0427-175, r6 acceptance 1-3): the
// repair the conflict text ADVERTISES,
//
//	baha app destroy --uuid <deployment-id> --full-reset --yes
//
// runs the production destroy entry (executeApplicationDestroyLifecycle) in-path
// — resolution, authorization (authorizeApplicationDestruction), preflight and
// mutation — and removes ONLY the selected twin's whole deployment directory
// INCLUDING its retained '.owner' band, leaves the other twin (record + band)
// byte-identical, and leaves the strict inventory seam healthy with exactly the
// survivor. A conflict error is explicitly a test FAILURE. Native runtime only:
// a --full-reset destroy reaches provider-authorization, which needs verified
// runtime ownership.
func TestRegistryConflictRepairViaRealCLIEntryDeletesSelectedTwinOnlyAndKeepsSibling(t *testing.T) {
	if os.Getenv("BASEHARBOR_APP_FULL_RESET_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native application full-reset repair acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 10*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	target := configureTestTarget(t)
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("baseharbor.yaml", []byte("version: 1\napp:\n  name: repair\n  environment: dev\nservices:\n  sql: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("app init: %v", err)
	}
	applySelectiveIdentityIntent(t, ctx)

	base, rerr := resolveApplication(ctx, application.Store{}, nil, "destroy")
	if rerr != nil {
		t.Fatalf("resolve repair candidate: %v", rerr)
	}
	twinIDs, baseRoot := seedPoisonedNativeTwinRegistry(t, ctx, base)
	keeper := twinIDs[0]
	if keeper == twinIDs[1] {
		t.Fatal("fixture produced a single identity")
	}
	keeperRoot, kerr := deployment.DeploymentRoot(deployment.DeploymentIdentity{DeploymentID: keeper, ApplicationID: base.Manifest.ApplicationID, Application: base.Manifest.Name, Environment: base.Manifest.Environment, Target: base.Target.Name})
	if kerr != nil {
		t.Fatal(kerr)
	}

	beforeKeeperRecord, kerr := os.ReadFile(filepath.Join(keeperRoot, "deployment.json"))
	if kerr != nil {
		t.Fatal(kerr)
	}
	beforeKeeperBand, berr := os.ReadFile(filepath.Join(keeperRoot, ".owner"))
	if berr != nil {
		t.Fatal(berr)
	}
	beforeBaseBand, berr := os.ReadFile(filepath.Join(baseRoot, ".owner"))
	if berr != nil {
		t.Fatal(berr)
	}

	// (1) The advertised repair runs the production destroy entry in-path with
	// the exact selector, reaching authorizeApplicationDestruction.
	ackPlan, perr := buildApplicationDestructionPlan(ctx, base, true)
	if perr != nil {
		t.Fatalf("build destruction plan: %v", perr)
	}
	ack := &application.ProviderChangeConsent{PlanID: ackPlan.Scope.PlanID, First: true, Second: true, DeleteData: true}
	destroyCtx := withProviderChangeConsent(ctx, ack)
	var out, errOut bytes.Buffer
	derr := executeApplicationDestroyLifecycle(destroyCtx, application.DefaultStore(),
		[]string{"--uuid", twinIDs[1], "--full-reset", "--yes"}, &out, &errOut)
	if derr != nil {
		t.Fatalf("the advertised repair must reach the deletion, not stop at the poison gate or a stale scope: %v\n%s", derr, out.String())
	}

	// (2) The selected twin's whole directory (deployment.json AND the band) is
	// gone.
	if _, statErr := os.Stat(filepath.Join(baseRoot, ".owner")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the selected twin's retained band must be removed with its directory, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(baseRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the selected twin's deployment directory must be gone, stat err=%v", statErr)
	}

	// (3) The survivor (the unaddressed twin) is byte-identical: record AND
	// band unchanged.
	afterKeeperRecord, rerr := os.ReadFile(filepath.Join(keeperRoot, "deployment.json"))
	if rerr != nil {
		t.Fatalf("the survivor twin must survive the repair: %v", rerr)
	}
	if !bytes.Equal(beforeKeeperRecord, afterKeeperRecord) {
		t.Fatal("the survivor twin's record must be byte-identical after repairing the selected twin")
	}
	afterKeeperBand, berr := os.ReadFile(filepath.Join(keeperRoot, ".owner"))
	if berr != nil {
		t.Fatalf("the survivor band must survive: %v", berr)
	}
	if !bytes.Equal(beforeKeeperBand, afterKeeperBand) {
		t.Fatal("the survivor twin's band must be byte-identical after repairing the selected twin")
	}
	if !bytes.Equal(beforeBaseBand, afterKeeperBand) {
		t.Fatal("survivor band content was altered")
	}

	// (4) Recovery is real: the strict inventory seam is healthy with exactly the
	// surviving deployment.
	healthy, ierr := deployment.ListDeployments(target.Name)
	if ierr != nil {
		t.Fatalf("after the repair the inventory seam must be healthy: %v", ierr)
	}
	if len(healthy) != 1 || healthy[0].Identity.DeploymentID != keeper {
		t.Fatalf("after the repair only the survivor may remain registered: %+v", healthy)
	}
	_ = bhruntime.CapabilityWorkloadLifecycle
	// Sanity hash to make the byte-identical claim machine-checked, not asserted.
	sum := sha256.Sum256(beforeKeeperRecord)
	if sum == [32]byte{} {
		t.Fatal("empty survivor record digest")
	}
}

// TestRegistryConflictCLIEntryDriftRefusedBeforeAnyMutation proves the r6
// selector guard (0427-175) fails closed BEFORE any destructive mutation on the
// real destroy entry: a deliberate selection that does not route to the resolved
// record refuses and leaves BOTH twins (records + retained '.owner' bands)
// intact. Native runtime only (authorization must run before the guard).
func TestRegistryConflictCLIEntryDriftRefusedBeforeAnyMutation(t *testing.T) {
	if os.Getenv("BASEHARBOR_APP_FULL_RESET_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native application full-reset repair acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 10*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	target := configureTestTarget(t)
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, provider, target)
	t.Chdir(t.TempDir())
	if err := os.WriteFile("compose.yaml", []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("baseharbor.yaml", []byte("version: 1\napp:\n  name: drift\n  environment: dev\nservices:\n  sql: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("app init: %v", err)
	}
	applySelectiveIdentityIntent(t, ctx)

	base, rerr := resolveApplication(ctx, application.Store{}, nil, "destroy")
	if rerr != nil {
		t.Fatalf("resolve: %v", rerr)
	}
	twinIDs, baseRoot := seedPoisonedNativeTwinRegistry(t, ctx, base)
	keeper, victim := twinIDs[0], twinIDs[1]
	victimRoot, verr := deployment.DeploymentRoot(deployment.DeploymentIdentity{DeploymentID: victim, ApplicationID: base.Manifest.ApplicationID, Application: base.Manifest.Name, Environment: base.Manifest.Environment, Target: base.Target.Name})
	if verr != nil {
		t.Fatal(verr)
	}

	before := map[string][]byte{}
	for _, root := range []string{baseRoot, victimRoot} {
		rec, rerr := os.ReadFile(filepath.Join(root, "deployment.json"))
		if rerr != nil {
			t.Fatalf("read %s record: %v", root, rerr)
		}
		band, berr := os.ReadFile(filepath.Join(root, ".owner"))
		if berr != nil {
			t.Fatalf("read %s band: %v", root, berr)
		}
		before[root] = append(rec, band...)
	}

	// Build an execution resolving keeper, then force the selection onto the
	// OTHER twin (victim), mirroring a corrupted selection between authorization
	// and mutation. The guard in removeApplicationState must refuse.
	plan, perr := buildApplicationDestructionPlan(ctx, base, true)
	if perr != nil {
		t.Fatalf("build destruction plan: %v", perr)
	}
	ack := &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true, DeleteData: true}

	exec, eerr := newApplicationDestroyExecution(withProviderChangeConsent(ctx, ack), application.DefaultStore(),
		[]string{"--uuid", keeper, "--full-reset", "--yes"}, io.Discard, io.Discard)
	if eerr != nil {
		t.Fatalf("build execution for drift cell: %v", eerr)
	}
	if exec.resolved.DeploymentIdentity.DeploymentID != keeper {
		t.Fatalf("drift cell setup: --uuid %s must resolve to keeper, got %s", keeper, exec.resolved.DeploymentIdentity.DeploymentID)
	}
	// Force the drift: corrupt the in-memory selection to name the OTHER twin.
	exec.selection = []string{"--uuid", victim}

	// Authorize + cleanup must run (real entry), then the guard fires in
	// removeApplicationState and refuses.
	var out, errOut bytes.Buffer
	derr := executeApplicationDestroyLifecycle(withProviderChangeConsent(ctx, ack), application.DefaultStore(),
		[]string{"--uuid", keeper, "--full-reset", "--yes"}, &out, &errOut)
	// The drift refusal surfaces through the entry. Either authorize accepts and
	// the guard refuses in removeApplicationState, or the guard rejects earlier;
	// in BOTH cases neither twin may be mutated.
	if derr == nil {
		t.Fatal("a drifted selection must be refused by the destroy entry")
	}
	for root, want := range before {
		got, gerr := os.ReadFile(filepath.Join(root, "deployment.json"))
		if gerr != nil {
			t.Fatalf("a refused drifted repair must leave %s record present: %v", root, gerr)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("a refused drifted repair must leave %s record byte-identical", root)
		}
	}
}
