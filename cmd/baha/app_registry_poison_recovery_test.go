package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytes"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"io"
	"sort"
)

// seedPoisonedTwinRegistry reproduces the hp4 BF-01 end state on the CURRENT
// binary: one application_id claimed by two deployment records of the same
// Target and environment. The fixed write seam refuses to create that shape,
// so the twins are persisted by direct file write — the state a target
// poisoned by an older binary really holds.
func seedPoisonedTwinRegistry(t *testing.T) (target deployment.ResolvedTarget, twinIDs [2]string) {
	t.Helper()
	target = configureTestTarget(t)
	appID := application.MustNewApplicationID()
	// One random identity per twin (as the poisoning race produces), so the two
	// records differ in their deployment id but share application_id, name and
	// environment — the exact shape the ambiguity guard blocks.
	for i := 0; i < 2; i++ {
		id, err := deployment.NewDeploymentIdentity(target.Name, appID, "twin", "dev")
		if err != nil {
			t.Fatal(err)
		}
		twinIDs[i] = id.DeploymentID
		root, err := deployment.DeploymentRoot(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		manifest := application.WithWorkloadComponents(application.New("twin", "dev", false, false, false), "twin-api")
		manifest.ApplicationID = appID
		intent, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		record := deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: id,
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: filepath.Join(root, "source")},
			Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: "docker"},
			Observed: deployment.ObservedDeployment{State: "created"},
		}
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "deployment.json"), append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The display order is the sorted id order, so callers can address the
	// twins deterministically by name without depending on random ids.
	sort.Strings(twinIDs[:])
	return target, twinIDs
}

// assertRegistryConflictError pins the ONE error shape every planning path
// must produce on a poisoned Target: the dedicated cause code, the conflict
// classification and — per lesson 162 — a Next line that names the WORKING
// repair verbatim, not a healing that does not heal.
func assertRegistryConflictError(t *testing.T, err error, twinIDs [2]string) {
	t.Helper()
	var failure *machine.Error
	if !errors.As(err, &failure) {
		t.Fatalf("planning path surfaced an unstructured error: %v", err)
	}
	if failure.CauseCode != "provider_consumer_inventory_invalid" {
		t.Fatalf("poisoned registry must keep the inventory cause code, got %q (%v)", failure.CauseCode, err)
	}
	joined := failure.Message + " " + failure.Next
	details, _ := failure.Details.(map[string]string)
	for _, detail := range details {
		joined += " " + detail
	}
	if !strings.Contains(joined, twinIDs[0]) || !strings.Contains(joined, twinIDs[1]) {
		t.Fatalf("the conflict must name BOTH colliding deployment uuids for the --uuid repair: %v", failure)
	}
	if !strings.Contains(joined, "baha app destroy --uuid") || !strings.Contains(joined, "--full-reset") {
		t.Fatalf("the conflict must name the verified repair command (destroy --uuid ... --full-reset): %v", failure)
	}
	if !errors.Is(err, deployment.ErrRegistryConflict) && !errors.Is(failure.Cause, deployment.ErrRegistryConflict) {
		t.Fatalf("the machine error must carry the registry-conflict sentinel: %v", err)
	}
}

// TestPlanningPathsOnPoisonedTargetNameTheWorkingRepair is the recovery-face
// regression (0427-175 acceptance (2), hp4 BF-01/BF-10): on a target poisoned
// by the pre-fix double registration, EVERY planning command fails closed
// with one clear error that names both colliding uuids and the exact working
// repair.
func TestPlanningPathsOnPoisonedTargetNameTheWorkingRepair(t *testing.T) {
	target, twinIDs := seedPoisonedTwinRegistry(t)

	// A healthy third application's plan goes through the shared inventory
	// seam too: the poison is Target-wide, which is why the repair must name
	// the two colliding records explicitly.
	thirdID, err := deployment.NewDeploymentIdentity(target.Name, application.MustNewApplicationID(), "healthy", "dev")
	if err != nil {
		t.Fatal(err)
	}
	thirdRoot, err := deployment.DeploymentRoot(thirdID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(thirdRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	thirdManifest := application.WithWorkloadComponents(application.New("healthy", "dev", false, false, false), "healthy-api")
	thirdManifest.ApplicationID = thirdID.ApplicationID
	intent, err := json.Marshal(thirdManifest)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: thirdID,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: filepath.Join(thirdRoot, "source")},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: "docker"},
		Observed: deployment.ObservedDeployment{State: "created"},
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(thirdRoot, "deployment.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolvedInventoryProbe(t, target, thirdManifest)
	if err != nil {
		t.Fatal(err)
	}
	_, planErr := resolveApplicationProviderPlan(resolved, nil)
	assertRegistryConflictError(t, planErr, twinIDs)

	if _, listErr := deployment.ListDeployments(target.Name); !errors.Is(listErr, deployment.ErrRegistryConflict) {
		t.Fatalf("the inventory seam must expose the sentinel: %v", listErr)
	}
}

// TestPoisonedTargetRemainsReachableByUUID proves the repair path the error
// text advertises actually exists on the SAME binary (lesson 162): the
// tolerant display listing still shows both twins with their uuids, and the
// --uuid selector reaches one record — where the NAME selector is blocked by
// the ambiguity guard.
func TestPoisonedTargetRemainsReachableByUUID(t *testing.T) {
	target, twinIDs := seedPoisonedTwinRegistry(t)

	records, _, err := deployment.ListDeploymentsForDisplay(target.Name)
	if err != nil {
		t.Fatalf("the display listing must tolerate the poison so the operator can see it: %v", err)
	}
	var seen [2]bool
	for _, record := range records {
		switch record.Identity.DeploymentID {
		case twinIDs[0]:
			seen[0] = true
		case twinIDs[1]:
			seen[1] = true
		}
	}
	if !seen[0] || !seen[1] {
		t.Fatalf("app list must show both colliding twins: %v", seen)
	}

	// A NAME-addressed command refuses and names both twins: the ambiguity
	// guard (0427-175) routes the operator onto the uuid path, which is the
	// only unambiguous identity on the poisoned Target.
	_, nameErr := resolveApplication(t.Context(), application.Store{}, []string{"twin"}, "status")
	if nameErr == nil {
		t.Fatal("name resolution must not silently pick one of two twins")
	}
	if !strings.Contains(nameErr.Error(), twinIDs[0]) || !strings.Contains(nameErr.Error(), twinIDs[1]) {
		t.Fatalf("the name refusal must name both colliding deployments: %v", nameErr)
	}

	// --uuid addresses exactly one twin, bypassing the NAME filter — the
	// command shape the guard itself advertises: 'baha app <cmd> --uuid
	// <deployment-id>'.
	resolved, resolveErr := resolveApplication(t.Context(), application.Store{}, []string{"--uuid", twinIDs[0]}, "status")
	if resolveErr != nil {
		if !errors.Is(resolveErr, deployment.ErrRegistryConflict) {
			t.Fatalf("the --uuid repair path must route to the twin or name the structured conflict, got %v", resolveErr)
		}
		for _, id := range twinIDs {
			if !strings.Contains(resolveErr.Error(), id) {
				t.Fatalf("the conflict must name twin %s: %v", id, resolveErr)
			}
		}
		return
	}
	if resolved.DeploymentIdentity.DeploymentID != twinIDs[0] {
		t.Fatalf("--uuid routed to the wrong record: %v", resolved.DeploymentIdentity)
	}
}

// poisonTwinFixture is the poison state of seedPoisonedTwinRegistry plus the
// one thing the plain seed deliberately lacks: a real repository source for the
// twins. Both twins claim one application_id through that SAME repository (that
// is what the double registration does), so the destroy path resolves them
// through its production repository branch and the mutation reaches the real
// DeleteDeploymentRecord seam.
func poisonTwinFixture(t *testing.T) (target deployment.ResolvedTarget, twinIDs [2]string, roots map[string]string) {
	t.Helper()
	target = configureTestTarget(t)
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, application.RepositoryManifestName)
	appID := application.MustNewApplicationID()
	manifest := application.WithWorkloadComponents(application.New("twin", "dev", false, false, false), "twin-api")
	manifest.ApplicationID = appID
	if err := os.WriteFile(manifestPath, []byte("name: twin\nenvironment: dev\nservices: {}\nworkload:\n  components:\n    - name: twin-api\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	roots = map[string]string{}
	for i := 0; i < 2; i++ {
		id, err := deployment.NewDeploymentIdentity(target.Name, appID, "twin", "dev")
		if err != nil {
			t.Fatal(err)
		}
		twinIDs[i] = id.DeploymentID
		root, err := deployment.DeploymentRoot(id)
		if err != nil {
			t.Fatal(err)
		}
		roots[id.DeploymentID] = root
		if err := os.MkdirAll(filepath.Join(root, "source"), 0o700); err != nil {
			t.Fatal(err)
		}
		record := deployment.DeploymentRecord{
			Version:  deployment.DeploymentRecordVersion,
			Identity: id,
			Source:   deployment.DeploymentSource{Kind: "repository", Repository: repo, Manifest: manifestPath},
			Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider},
			Observed: deployment.ObservedDeployment{State: "created"},
		}
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "deployment.json"), append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(twinIDs[:])
	return target, twinIDs, roots
}

// TestRegistryConflictRepairCommandDeletesSelectedTwinOnly is the executable
// recovery regression (0427-175 acceptance (2), lesson 162): the repair the
// conflict text ADVERTISES,
//
//	baha app destroy --uuid <deployment-id> --full-reset
//
// must reach the production deletion seam (deployment.DeleteDeploymentRecord)
// with the identity it resolved, and remove ONLY the selected twin. A conflict
// error is explicitly NOT a passing outcome; the cell fails if the advertised
// command stops at the poison gate instead of deleting.
func TestRegistryConflictRepairCommandDeletesSelectedTwinOnly(t *testing.T) {
	target, twinIDs, roots := poisonTwinFixture(t)
	victim, keeper := twinIDs[0], twinIDs[1]

	// The poison is really in place: the strict inventory seam refuses, the
	// tolerant display listing still shows BOTH twins, and a NAME-addressed
	// destroy is blocked — the state the operator is standing in when they
	// read the recovery hint (0427-175, hp4 BF-01).
	if _, err := deployment.ListDeployments(target.Name); !errors.Is(err, deployment.ErrRegistryConflict) {
		t.Fatalf("precondition: the registry must be poisoned, got %v", err)
	}
	before, _, err := deployment.ListDeploymentsForDisplay(target.Name)
	if err != nil {
		t.Fatalf("precondition: the display listing must tolerate the poison: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("precondition: the listing must show both twins, got %+v", before)
	}
	if _, nameErr := resolveApplication(t.Context(), application.Store{}, []string{"twin"}, "destroy"); nameErr == nil {
		t.Fatal("precondition: a NAME-addressed destroy must not pick one of two twins")
	}
	keeperBefore, err := os.ReadFile(filepath.Join(roots[keeper], "deployment.json"))
	if err != nil {
		t.Fatal(err)
	}

	// (1) Parsing and routing: the exact advertised arguments must reach the
	// selected twin, and the selection must survive into the execution that
	// authorizes, preflights and mutates. This fails if --uuid parsing or
	// routing is disabled (0427-175 acceptance (1)).
	args := []string{"--uuid", victim, "--full-reset", "--yes"}
	execution, err := newApplicationDestroyExecution(t.Context(), application.Store{}, args, io.Discard, io.Discard)
	if err != nil {
		if deployment.IsRegistryConflict(err) {
			t.Fatalf("the advertised repair must reach the deletion, not stop at the poison gate: %v", err)
		}
		t.Fatalf("the advertised repair command failed: %v", err)
	}
	if execution.resolved.DeploymentIdentity.DeploymentID != victim {
		t.Fatalf("the repair routed at the wrong deployment: got %q want %q", execution.resolved.DeploymentIdentity.DeploymentID, victim)
	}
	if !execution.fullReset || !execution.confirmed {
		t.Fatalf("--full-reset/--yes must survive the uuid routing: fullReset=%t confirmed=%t", execution.fullReset, execution.confirmed)
	}
	if err := execution.runPreflight(t.Context()); err != nil {
		t.Fatalf("the repair must pass the production preflight of the SELECTED twin: %v", err)
	}

	// (2) Mutation: the production removal seam deletes the selected twin's
	// record and nothing else.
	if err := execution.removeApplicationState(); err != nil {
		t.Fatalf("the repair's state removal must succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(roots[victim], "deployment.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the selected twin's record must be gone, stat err=%v", err)
	}
	keeperAfter, err := os.ReadFile(filepath.Join(roots[keeper], "deployment.json"))
	if err != nil {
		t.Fatalf("the sibling twin must survive the repair: %v", err)
	}
	if !bytes.Equal(keeperBefore, keeperAfter) {
		t.Fatal("the sibling twin must be byte-identical after repairing the selected twin")
	}

	// (3) Recovery is real: the strict inventory seam is healthy again and the
	// listing shows exactly the survivor, i.e. 'baha app list' works.
	healthy, err := deployment.ListDeployments(target.Name)
	if err != nil {
		t.Fatalf("after the repair the inventory seam must be healthy: %v", err)
	}
	if len(healthy) != 1 || healthy[0].Identity.DeploymentID != keeper {
		t.Fatalf("after the repair only the sibling may remain registered: %+v", healthy)
	}
	records, _, err := deployment.ListDeploymentsForDisplay(target.Name)
	if err != nil {
		t.Fatalf("the listing must stay readable after the repair: %v", err)
	}
	if len(records) != 1 || records[0].Identity.DeploymentID != keeper {
		t.Fatalf("after the repair only the sibling may remain registered: %+v", records)
	}
}

// TestRegistryConflictRepairOmittedOrInvalidUUIDFailsClosed keeps the repair
// honest: without a usable selector the command must never guess a twin, and
// it must leave both records in place.
func TestRegistryConflictRepairOmittedOrInvalidUUIDFailsClosed(t *testing.T) {
	target, twinIDs, roots := poisonTwinFixture(t)
	_ = target

	cases := map[string][]string{
		"no selector at all":      {"--full-reset", "--yes"},
		"--uuid without a value":  {"--uuid", "--full-reset"},
		"empty uuid value":        {"--uuid", "", "--full-reset", "--yes"},
		"uuid and NAME together":  {"twin", "--uuid", twinIDs[0]},
		"conflicting uuid values": {"--uuid", twinIDs[0], "--uuid", twinIDs[1]},
	}
	for name, args := range cases {
		if _, err := newApplicationDestroyExecution(t.Context(), application.Store{}, args, io.Discard, io.Discard); err == nil {
			t.Fatalf("%s: must fail closed, the command resolved an execution", name)
		}
	}
	for _, id := range twinIDs {
		if _, err := os.Stat(filepath.Join(roots[id], "deployment.json")); err != nil {
			t.Fatalf("a refused repair invocation must not remove twin %s: %v", id, err)
		}
	}
}

// TestRegistryConflictDetectionIsOrderIndependent locks the detection rule
// shared by the write seam and the inventory seam: one group per
// (application_id, environment), ids sorted, empty application ids never a
// collision.
func TestRegistryConflictDetectionIsOrderIndependent(t *testing.T) {
	target, twinIDs := seedPoisonedTwinRegistry(t)
	records, _, err := deployment.ListDeploymentsForDisplay(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	err = deployment.RefuseApplicationIDConflictOnDisplay(target.Name, records)
	if !errors.Is(err, deployment.ErrRegistryConflict) {
		t.Fatalf("expected the conflict sentinel from the shared detector, got %v", err)
	}
	var conflict *deployment.RegistryConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("the detector must report the structured conflict: %T", err)
	}
	if len(conflict.DeploymentIDs) != 2 || conflict.DeploymentIDs[0] > conflict.DeploymentIDs[1] {
		t.Fatalf("the detector must name both twins deterministically: %v", conflict.DeploymentIDs)
	}
	for _, id := range twinIDs {
		if !strings.Contains(err.Error(), id) {
			t.Fatalf("the conflict must name twin %s: %v", id, err)
		}
	}
}

// resolvedInventoryProbe builds a resolvedApplication for a healthy manifest
// on target without routing through the (poisoned) name filter, so the cell
// exercises the shared provider-plan inventory seam directly.
func resolvedInventoryProbe(t *testing.T, target deployment.ResolvedTarget, m application.Manifest) (resolvedApplication, error) {
	t.Helper()
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		return resolvedApplication{}, err
	}
	if err := os.WriteFile(filepath.Join(root, "provider-registry.json"), []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
	if err != nil {
		return resolvedApplication{}, err
	}
	deploymentRoot, err := deployment.DeploymentRoot(id)
	if err != nil {
		return resolvedApplication{}, err
	}
	return resolvedApplication{
		Target:              target,
		DeploymentIdentity:  id,
		Manifest:            m,
		RepositoryRoot:      deploymentRoot,
		TargetStateRoot:     root,
		DeploymentStateRoot: deploymentRoot,
		ManifestPath:        filepath.Join(deploymentRoot, application.RepositoryManifestName),
		Store:               application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: target.Name},
		SourceAvailable:     true,
	}, nil
}
