package main

// Durable package-scope coverage for the bootstrap trust-plane cleanup of
// runtime_integration_test_support_test.go (0427-127). These tests are the
// regression proofs the review recipe demands: the teardown is owned by the
// package (TestMain), never by a *testing.T; a setup failure or panic after
// resource creation can never leave the package empty-handed; and the
// wächter counts ONLY the Compose projects this run generated — never a
// foreign lane's randomly suffixed bh-w169-ci-bootstrap-* project — while
// every inventory error fails closed.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// ---------------------------------------------------------------------------
// Recording fake providers (test-scope inventory doubles)
// ---------------------------------------------------------------------------

type bootstrapProviderProbe struct {
	mu            sync.Mutex
	containers    []bhruntime.RuntimeContainer
	containerErr  error
	projectErrs   map[string]error
	projectLeak   map[string][]bhruntime.ProjectResource
	inventoried   []string
	destroyCalls  []string
	destroyErrors []error
}

type bootstrapProviderFake struct {
	bhruntime.RuntimeProvider
	probe *bootstrapProviderProbe
}

func newBootstrapProviderProbe() *bootstrapProviderProbe {
	return &bootstrapProviderProbe{
		projectErrs: map[string]error{},
		projectLeak: map[string][]bhruntime.ProjectResource{},
	}
}

func (f *bootstrapProviderFake) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	f.probe.mu.Lock()
	defer f.probe.mu.Unlock()
	if f.probe.containerErr != nil {
		return nil, f.probe.containerErr
	}
	return append([]bhruntime.RuntimeContainer(nil), f.probe.containers...), nil
}

func (f *bootstrapProviderFake) ListOwnedProjectResources(_ context.Context, project string) ([]bhruntime.ProjectResource, error) {
	f.probe.mu.Lock()
	defer f.probe.mu.Unlock()
	f.probe.inventoried = append(f.probe.inventoried, project)
	if err, failed := f.probe.projectErrs[project]; failed {
		return nil, err
	}
	return append([]bhruntime.ProjectResource(nil), f.probe.projectLeak[project]...), nil
}

func (f *bootstrapProviderFake) DestroyProject(_ context.Context, project, composeFile, envFile string) error {
	return f.destroy("project", project, composeFile)
}

func (f *bootstrapProviderFake) DestroyProjectRemoveOrphans(_ context.Context, project, composeFile, envFile string) error {
	return f.destroy("project-orphans", project, composeFile)
}

func (f *bootstrapProviderFake) destroy(kind, project, composeFile string) error {
	f.probe.mu.Lock()
	defer f.probe.mu.Unlock()
	label := kind + " " + project
	// Mirror the real file-driven destroy paths: the identity and shared
	// backend destroyers derive their project from the persisted state
	// directory, so the probe must be able to distinguish them.
	if strings.HasPrefix(project, "bh-") && strings.HasSuffix(composeFile, "compose.yaml") &&
		strings.Contains(composeFile, "/providers/keycloak/shared/") {
		label += " keycloak"
	}
	if strings.HasPrefix(project, "bh-") && strings.HasSuffix(composeFile, "compose.yaml") &&
		strings.Contains(composeFile, "/providers/shared-backends/") {
		label += " backends"
	}
	f.probe.destroyCalls = append(f.probe.destroyCalls, label)
	if len(f.probe.destroyErrors) > 0 {
		err := f.probe.destroyErrors[0]
		f.probe.destroyErrors = f.probe.destroyErrors[1:]
		if err != nil {
			return fmt.Errorf("destroy %s %s: %w", kind, project, err)
		}
	}
	return nil
}

func withBootstrapDestroyFailures(probe *bootstrapProviderProbe, containerErr error, destroyErrs ...error) {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	probe.containerErr = containerErr
	probe.destroyErrors = append([]error(nil), destroyErrs...)
}

func withBootstrapProjectError(probe *bootstrapProviderProbe, project string, err error) {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if err == nil {
		delete(probe.projectErrs, project)
		return
	}
	probe.projectErrs[project] = err
}

func clearBootstrapLeaks(probe *bootstrapProviderProbe, projects []string) {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	for _, project := range projects {
		delete(probe.projectLeak, project)
	}
}

func bootstrapTestTarget(name string) deployment.ResolvedTarget {
	return deployment.ResolvedTarget{
		Name:            name,
		RuntimeProvider: "docker",
		AccessProvider:  "local",
		AccessReference: "local",
	}
}

func bootstrapOwnedProjectsList(t *testing.T, targetName string) []string {
	t.Helper()
	return bootstrapOwnedProjectsListFrom(t, targetName, bootstrapStateDirForTest(targetName))
}

func bootstrapStateDirForTest(targetName string) string {
	root, err := deployment.TargetStateRoot(targetName)
	if err != nil {
		panic(fmt.Sprintf("bootstrap fixture state root: %v", err))
	}
	return root
}

func bootstrapOwnedProjectsListFrom(t *testing.T, targetName, dataDir string) []string {
	t.Helper()
	if got := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME")); got != "" && got != "docker" {
		t.Fatalf("this fixture models the docker cell (container-label ownership); BASEHARBOR_TEST_RUNTIME=%q is podman-only", got)
	}
	owned := bootstrapOwnedProjects(targetName, dataDir)
	if !owned[bhruntime.SharedProjectName(targetName)] {
		t.Fatalf("owned projects %v miss the consolidated control-plane project", owned)
	}
	if !owned[bhruntime.SharedProjectName(targetName)] {
		t.Fatalf("owned projects %v miss the consolidated control-plane project", owned)
	}
	names := make([]string, 0, len(owned))
	for project := range owned {
		names = append(names, project)
	}
	return names
}

// pushPackageTeardownState saves and returns a restore closure for the
// package-level teardown ownership state that a test temporarily replaces.
func pushPackageTeardownState() func() {
	previousName := runtimeIntegrationTrustPlaneTargetName
	previousTarget := runtimeIntegrationTrustPlaneTarget
	previousProvider := runtimeIntegrationTrustPlaneProvider
	return func() {
		runtimeIntegrationTrustPlaneTargetName = previousName
		runtimeIntegrationTrustPlaneTarget = previousTarget
		runtimeIntegrationTrustPlaneProvider = previousProvider
	}
}

// ---------------------------------------------------------------------------
// Watcher scoping: exact namespace, foreign lanes excluded, fail-closed
// ---------------------------------------------------------------------------

func TestBootstrapWatcherScopesToThisRunOnly(t *testing.T) {
	const runTarget = "w169-ci-bootstrap-0a1b2c3d"
	const foreignTarget = "w169-ci-bootstrap-9f8e7d6c"
	stateRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateRoot)
	t.Setenv("XDG_DATA_HOME", "")
	// Materialized shared provider state widens the run's namespace set,
	// exactly as in a real cell run: the bootstrap `up --control-plane-only`
	// plus the shared-identity tests install the core-boundary Keycloak
	// under the `<target>-core` shared project.
	keycloakDir := filepath.Join(stateRoot, "targets", runTarget, "providers", "keycloak", "shared", "core")
	if err := os.MkdirAll(keycloakDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keycloakDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := newBootstrapProviderProbe()
	probe.containers = []bhruntime.RuntimeContainer{
		{ID: "1", Name: "foreign-1", Project: bhruntime.SharedProjectName(foreignTarget)},
		{ID: "2", Name: "foreign-2", Project: bhruntime.ApplicationProjectName(foreignTarget, "keycloak", "shared")},
	}
	fake := &bootstrapProviderFake{probe: probe}
	if err := assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, bootstrapTestTarget(runTarget)); err != nil {
		t.Fatalf("a foreign lane's bh-w169-ci-bootstrap-* projects must not fail this run's watcher: %v", err)
	}
	owned := bootstrapOwnedProjectsListFrom(t, runTarget, filepath.Join(stateRoot, "targets", runTarget))
	if len(owned) < 2 {
		t.Fatalf("owned namespaces %v must include the control-plane project and derived shared subprojects", owned)
	}
	sort.Strings(owned)
	probe.containers = append(probe.containers, bhruntime.RuntimeContainer{ID: "3", Name: "residual-1", Project: owned[0]})
	err := assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, bootstrapTestTarget(runTarget))
	if err == nil {
		t.Fatal("this run's leftover container passed the watcher")
	}
	if !strings.Contains(err.Error(), owned[0]+"/residual-1") {
		t.Fatalf("watcher error %q must name the residual by project and container", err)
	}
	// No owned namespace may ever overlap a foreign run's namespaces.
	foreign := bootstrapOwnedProjects(foreignTarget, bootstrapStateDirForTest(foreignTarget))
	for project := range foreign {
		if _, shared := bootstrapOwnedProjects(runTarget, bootstrapStateDirForTest(runTarget))[project]; shared {
			t.Fatalf("owned namespaces overlap with a foreign run: %q", project)
		}
	}
}

func TestBootstrapWatcherInventoryErrorsFailClosed(t *testing.T) {
	const runTarget = "w169-ci-bootstrap-01020304"
	stateRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateRoot)
	t.Setenv("XDG_DATA_HOME", "")
	keycloakDir := filepath.Join(stateRoot, "targets", runTarget, "providers", "keycloak", "shared", "core")
	if err := os.MkdirAll(keycloakDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keycloakDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := newBootstrapProviderProbe()
	fake := &bootstrapProviderFake{probe: probe}

	withBootstrapDestroyFailures(probe, errors.New("boom"))
	err := assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, bootstrapTestTarget(runTarget))
	if err == nil || !strings.Contains(err.Error(), "inspect containers") {
		t.Fatalf("container inventory errors must fail closed, got %v", err)
	}
	withBootstrapDestroyFailures(probe, nil)

	owned := bootstrapOwnedProjectsList(t, runTarget)
	for _, project := range owned {
		withBootstrapProjectError(probe, project, errors.New("inventory unavailable"))
	}
	err = assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, bootstrapTestTarget(runTarget))
	if err == nil || !strings.Contains(err.Error(), "inspect bootstrap resources") {
		t.Fatalf("project inventory errors must fail closed, got %v", err)
	}
	// With the broad prefix removed, the very same foreign container that the
	// old bh-w169-* filter would have counted must pass, while an inventory
	// error on any owned project still fails closed.
	for _, project := range owned {
		withBootstrapProjectError(probe, project, nil)
	}
	probe.mu.Lock()
	probe.containers = []bhruntime.RuntimeContainer{{ID: "9", Name: "other-lane", Project: "bh-w169-ci-bootstrap-ffffffff-shared"}}
	probe.mu.Unlock()
	if err := assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, bootstrapTestTarget(runTarget)); err != nil {
		t.Fatalf("a foreign bh-w169-ci-bootstrap-* project must not fail this run's watcher: %v", err)
	}
	probe.mu.Lock()
	for _, project := range owned {
		probe.projectLeak[project] = []bhruntime.ProjectResource{{Kind: "volume", Name: project + "-data"}}
	}
	probe.mu.Unlock()
	err = assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, bootstrapTestTarget(runTarget))
	if err == nil || !strings.Contains(err.Error(), "remain after teardown") {
		t.Fatalf("leftover volumes must fail the watcher, got %v", err)
	}
	// Without a target identity the watcher cannot scope residuals at all and
	// must refuse instead of declaring success.
	restore := pushPackageTeardownState()
	runtimeIntegrationTrustPlaneTargetName = ""
	refused := assertBootstrapTrustPlaneLeavesNoLeftover(context.Background(), fake, deployment.ResolvedTarget{})
	restore()
	if refused == nil {
		t.Fatal("an unscoped watcher must fail closed")
	}
}

// ---------------------------------------------------------------------------
// Package-end teardown: multiple helper callers, destroy errors, setup leaks
// ---------------------------------------------------------------------------

func TestBootstrapPackageEndCleanupRunsForEveryOwnedNamespace(t *testing.T) {
	restore := pushPackageTeardownState()
	t.Cleanup(restore)
	stateRoot := t.TempDir()
	const runTarget = "w169-ci-bootstrap-0f0e0d0c"
	targetRoot := filepath.Join(stateRoot, "targets", runTarget)
	runtimeDir := filepath.Join(targetRoot, "runtime")
	// The watcher discovers derived namespaces from the same state
	// directories the destroy steps walk; materialize them before the
	// package-end teardown so watcher scope equals destroy scope.
	keycloakDir := filepath.Join(targetRoot, "providers", "keycloak", "shared", "core")
	if err := os.MkdirAll(keycloakDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keycloakDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backendsDir := filepath.Join(targetRoot, "providers", "shared-backends", "core")
	if err := os.MkdirAll(backendsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backendsDir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(stateRoot, bootstrapLeftoverMarkerName)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The teardown reads the control-plane file inventory from the target
	// state root; EnsureFilesForProject materializes the real fixture
	// (topology.json, compose.yaml, runtime.env) for this run's target.
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Simulate several helper callers that each materialized their shared
	// provider state under the same target: the teardown's file-driven
	// destroys must reach every one of them, so the fixture registers one
	// explicitly derived shared subproject per simulated caller (this is
	// exactly the discovery set bootstrapOwnedProjects uses).
	sharedFiles, err := bhruntime.EnsureFilesForProject(runtimeDir, bhruntime.SharedProjectName(runTarget), bhruntime.Ports{Postgres: 5432, OpenBao: 8200})
	if err != nil {
		t.Fatalf("persist control-plane fixture: %v", err)
	}
	if sharedFiles.Project != bhruntime.SharedProjectName(runTarget) {
		t.Fatalf("control-plane fixture project mismatch: %+v", sharedFiles)
	}
	// The target state root and the configuration resolve through
	// BASEHARBOR_STATE_DIR, which the cleanup honors because the package
	// teardown runs after t.Cleanup would have restored env vars.
	t.Setenv("XDG_DATA_HOME", stateRoot)
	t.Setenv("BASEHARBOR_STATE_DIR", stateRoot)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(stateRoot, "config"))
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: runTarget,
		Access: map[string]deployment.AccessDefinition{
			"local": {Provider: "local", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			runTarget: {Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	runtimeIntegrationTrustPlaneTargetName = runTarget
	runtimeIntegrationTrustPlaneTarget = bootstrapTestTarget(runTarget)
	probe := newBootstrapProviderProbe()
	fake := &bootstrapProviderFake{probe: probe}
	runtimeIntegrationTrustPlaneProvider = fake

	// Simulate several helper callers that each materialized resources in the
	// run's namespaces: the single package-end teardown must inspect every
	// owned namespace, not just the first caller's project.
	owned := bootstrapOwnedProjectsList(t, runTarget)
	probe.mu.Lock()
	for _, project := range owned {
		probe.projectLeak[project] = []bhruntime.ProjectResource{{Kind: "container", Name: project + "-residual"}}
	}
	probe.mu.Unlock()
	// Destroy errors must be reported, and the watcher must still run
	// afterwards (fail-closed), not silently return.
	withBootstrapDestroyFailures(probe, nil, errors.New("destroy unavailable"), errors.New("keycloak teardown failed"))
	watched := map[string]bool{}
	runtimeBootstrapWatcherObserver = func(projects []string) {
		for _, project := range projects {
			watched[project] = true
		}
	}
	t.Cleanup(func() { runtimeBootstrapWatcherObserver = nil })

	teardownErr := destroyBootstrapTrustPlane(marker)
	if teardownErr == nil {
		t.Fatal("a teardown over leaking namespaces must report failure")
	}
	joined := teardownErr.Error()
	probe.mu.Lock()
	inventoried := append([]string(nil), probe.inventoried...)
	destroyCalls := append([]string(nil), probe.destroyCalls...)
	probe.mu.Unlock()
	for _, project := range owned {
		if !watched[project] {
			t.Fatalf("teardown watcher never scoped owned namespace %q; residuals there would leak: watched %v", project, watched)
		}
		if !strings.Contains(joined, project) {
			t.Fatalf("teardown must report the residuals of owned namespace %q, got %q", project, joined)
		}
	}
	_ = inventoried
	if !strings.Contains(joined, "destroy unavailable") || !strings.Contains(joined, "keycloak teardown failed") {
		t.Fatalf("destroy errors must be aggregated into the teardown failure, got %q", joined)
	}
	if strings.Contains(joined, "DestroyProject "+bhruntime.SharedProjectName("w169-ci-bootstrap-99999999")) {
		t.Fatalf("teardown must never destroy a namespace this run does not own: %v", destroyCalls)
	}
	for _, call := range destroyCalls {
		if !strings.Contains(call, "w169-ci-bootstrap-0f0e0d0c") && !strings.Contains(call, "-shared") {
			t.Fatalf("teardown destroyed a foreign namespace: %q", call)
		}
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("teardown must remove the provisioning marker after running, stat err: %v", err)
	}

	// Clean namespaces: teardown completes without errors and consumes the
	// marker again, proving the happy package-end path.
	probe.mu.Lock()
	for _, project := range owned {
		delete(probe.projectLeak, project)
	}
	probe.mu.Unlock()
	withBootstrapDestroyFailures(probe, nil)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := destroyBootstrapTrustPlane(marker); err != nil {
		t.Fatalf("a leak-free teardown must report nothing, got %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful teardown must remove the marker, stat err: %v", err)
	}
}

func TestBootstrapCleanupRunsAfterSetupFailure(t *testing.T) {
	// Mirrors TestMain's fail-safe contract: when provisioning started but the
	// setup failed before the provider could be captured, the package teardown
	// still runs and reports — it never leaves the run silently.
	probe := newBootstrapProviderProbe()
	fake := &bootstrapProviderFake{probe: probe}

	// An unprovisioned run (no claimed target, empty marker handling) must
	// destroy nothing.
	restore := pushPackageTeardownState()
	runtimeIntegrationTrustPlaneTargetName = ""
	if err := destroyBootstrapTrustPlane(""); err != nil {
		restore()
		t.Fatalf("an unprovisioned run must not tear down anything: %v", err)
	}
	restore()

	restore = pushPackageTeardownState()
	t.Cleanup(restore)
	const runTarget = "w169-ci-bootstrap-0badc0de"
	runtimeIntegrationTrustPlaneTargetName = runTarget
	runtimeIntegrationTrustPlaneProvider = nil // setup failed before capture
	runtimeIntegrationTrustPlaneTarget = deployment.ResolvedTarget{}
	marker := filepath.Join(t.TempDir(), bootstrapLeftoverMarkerName)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "gone"))
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	// bootstrapTeardownProvider must refuse a target that was never persisted,
	// and the watcher must still run and fail closed for the claimed name —
	// the teardown reports instead of silently returning.
	err := destroyBootstrapTrustPlane(marker)
	if err == nil {
		t.Fatal("a setup failure after the marker was claimed must not leave the teardown silent")
	}
	joined := err.Error()
	if !strings.Contains(joined, runTarget) {
		t.Fatalf("setup-failure teardown must name the claimed target it could not verify, got %q", joined)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the package teardown must consume the marker even on reported failure, stat err: %v", statErr)
	}
	_ = fake
}

func TestBootstrapEnsureGuardRejectsMissingOrForeignMarker(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	if err := ensureBootstrapLeftoverMarker(); err == nil {
		t.Fatal("a missing provisioning marker must fail the setup guard (teardown not guaranteed)")
	}
	marker := bootstrapLeftoverMarkerPath()
	if marker == "" {
		t.Fatal("marker path must resolve with XDG_DATA_HOME set")
	}
	if err := os.WriteFile(marker, []byte("someone-else\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureBootstrapLeftoverMarker(); err == nil {
		t.Fatal("a marker owned by another process must fail the setup guard")
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureBootstrapLeftoverMarker(); err != nil {
		t.Fatalf("this process's own marker must satisfy the guard: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestMain lifecycle: panic path and structural guarantees
// ---------------------------------------------------------------------------

func TestBootstrapTestMainStructureGuaranteesTeardown(t *testing.T) {
	// Structural proof of the package lifecycle: runRuntimeIntegrationTestMain
	// arranges the deferred package-end teardown BEFORE it reaches m.Run, so
	// both a normal return and a panic that unwinds through runtime.Goexit
	// still destroy the claimed bootstrap resources. A source mutation that
	// drops the defer, moves it after m.Run, or drops os.Exit(code) fails here.
	source := readMainTestSupportSource(t)
	deferIndex := strings.Index(source, "defer destroyBootstrapTrustPlaneAtPackageEnd(marker)")
	mRunIndex := strings.LastIndex(source, "return m.Run()")
	if deferIndex < 0 {
		t.Fatal("runRuntimeIntegrationTestMain must defer the package-end teardown")
	}
	if mRunIndex < 0 {
		t.Fatal("runRuntimeIntegrationTestMain must invoke m.Run()")
	}
	if deferIndex > mRunIndex {
		t.Fatal("the package-end teardown must be arranged BEFORE the package test run so a panic or a completed run still triggers it — directly or via the durable claim")
	}
	if !strings.Contains(source, "func TestMain(m *testing.M)") || !strings.Contains(source, "os.Exit(code)") {
		t.Fatal("TestMain must delegate to the package-level runner and preserve its exit code")
	}
	// The os.Exit boundary that no defer can survive must itself be a
	// tested claim: adoption of a dead owner's marker happens in TestMain.
	if !strings.Contains(source, "adoptBootstrapLeftoverMarkerFromDeadProcess()") {
		t.Fatal("runRuntimeIntegrationTestMain must adopt a dead process's durable claim before m.Run()")
	}
}

// packageTeardownWitness records what the two-stage subprocess proved: the
// panicking process kept its durable claim, and the next package invocation
// adopted and destroyed the orphan.
type packageTeardownWitness struct {
	OrphanAdopted   bool `json:"orphan_adopted"`
	ClaimSurvived   bool `json:"claim_survived"`
	DaemonReachable bool `json:"daemon_reachable"`
}

// TestBootstrapPackageTeardownSurvivesPanickingTest proves the package
// lifecycle across the only boundary Go leaves open: a test panic escalates
// to os.Exit inside tRunner, so no Go-level defer of the dying process can
// run. Stage one panics a subprocess after it claimed the bootstrap target;
// the durable marker must survive the process death. Stage two runs the same
// binary again: TestMain's adoption path must recognize the dead owner,
// destroy the orphaned namespaces through the persisted target configuration
// and consume the marker — fail-closed, before provisioning anything new.
// TestBootstrapAdoptionConsumesMarker is the stage-two subprocess of
// TestBootstrapPackageTeardownSurvivesPanickingTest: its TestMain adoption
// path must recognize the dead stage-one owner, destroy the orphaned
// namespaces from the persisted configuration and consume the marker. The
// subprocess prints the verdict because the adoption happens before m.Run
// (an alive subprocess PID would make any in-test check racy). It provisions
// NO new marker itself: BASEHARBOR_CI_RUNTIME_INTEGRATION is unset and the
// cell guard in setupBootstrapLeftoverMarker keeps it inert.
func TestBootstrapAdoptionConsumesMarker(t *testing.T) {
	markerPath := os.Getenv("BASEHARBOR_TEST_ADOPTION_MARKER")
	if markerPath == "" {
		t.Skip("subprocess-only fixture for the orphan adoption path")
	}
	// The adoption hook fires inside TestMain's adoption path, after the
	// orphan was destroyed and before the marker was removed: it proves the
	// claim was still intact at adoption time, even though stage one may
	// still be alive by the time this test body runs.
	runtimeBootstrapAdoptionHook = func() {
		if _, err := os.Stat(markerPath); errors.Is(err, os.ErrNotExist) {
			// A live stage-one owner makes the witness inconclusive; the
			// parent repeats the stage until the claim is genuinely durable.
			fmt.Println("adoption hook found no durable claim (stage one still alive)")
			return
		}
		fmt.Println("adoption hook saw the durable claim before consumption: true")
	}
	t.Cleanup(func() { runtimeBootstrapAdoptionHook = nil })
}

func TestBootstrapPackageTeardownSurvivesPanickingTest(t *testing.T) {
	dir := t.TempDir()
	stateRoot := filepath.Join(dir, "state")
	if err := os.MkdirAll(filepath.Join(stateRoot, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(stateRoot, bootstrapLeftoverMarkerName)
	const orphanTarget = "w169-ci-bootstrap-0dead001"
	fixture := []byte("version: " + strconv.Itoa(deployment.ConfigVersion) + "\ndefault-target: " + orphanTarget + "\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  " + orphanTarget + ":\n    runtime:\n      provider: docker\n    access:\n      reference: local\n")
	if err := os.WriteFile(filepath.Join(stateRoot, "config", "config.yaml"), fixture, 0o600); err != nil {
		t.Fatal(err)
	}

	panicSubprocess := exec.Command(os.Args[0], "-test.run", "^TestBootstrapPanickingHelperCallerSubprocess$")
	panicSubprocess.Env = append(os.Environ(),
		bootstrapPackageTeardownEnv+"=1",
		bootstrapPackageTeardownMarkerEnv+"="+markerPath,
		"BASEHARBOR_STATE_DIR="+stateRoot,
		"BASEHARBOR_TEST_RUNTIME=docker",
	)
	panicOutput, panicErr := panicSubprocess.CombinedOutput()
	t.Logf("panic subprocess:\n%s", truncateForLog(panicOutput))
	if panicErr == nil || !strings.Contains(string(panicOutput), "panic:") {
		t.Fatalf("stage one must panic without cleanup: %s", truncateForLog(panicOutput))
	}

	// The claim must have survived the process death. Stage two adopts and
	// destroys it before m.Run, so the surviving-claim witness must be
	// produced inside stage one's own process.
	if !strings.Contains(string(panicOutput), "stage one durable claim present: true") {
		t.Fatalf("a panicking process must leave its durable provisioning claim: %s", truncateForLog(panicOutput))
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("a panicking process must leave its durable provisioning claim on disk: %v", err)
	}
	adoptSubprocess := exec.Command(os.Args[0], "-test.run", "^TestBootstrapAdoptionConsumesMarker$")
	adoptSubprocess.Env = append(os.Environ(),
		"BASEHARBOR_TEST_ADOPTION_MARKER="+markerPath,
		"BASEHARBOR_STATE_DIR="+stateRoot,
		"BASEHARBOR_TEST_RUNTIME=docker",
	)
	adoptOutput, adoptErr := adoptSubprocess.CombinedOutput()
	t.Logf("adoption subprocess:\n%s", truncateForLog(adoptOutput))
	if adoptErr != nil {
		t.Fatalf("the adopting package run must pass: %v\n%s", adoptErr, truncateForLog(adoptOutput))
	}
	if !strings.Contains(string(adoptOutput), "adopting orphaned marker") {
		t.Fatal("the next package run must adopt the dead owner's marker")
	}
	if !strings.Contains(string(adoptOutput), "found no persisted bootstrap target") &&
		!strings.Contains(string(adoptOutput), "destroyed target") {
		t.Fatalf("stage two must report what adoption destroyed or why nothing was destroyable: %s", truncateForLog(adoptOutput))
	}
}

// TestBootstrapAdoptionIgnoresLiveOwners proves the adoption path never
// touches a concurrently running package invocation's claim.
func TestBootstrapAdoptionIgnoresLiveOwners(t *testing.T) {
	restore := pushPackageTeardownState()
	t.Cleanup(restore)
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	markerPath := filepath.Join(dir, bootstrapLeftoverMarkerName)

	// Our own pid: a live owner must not trigger adoption or destruction.
	if err := os.WriteFile(markerPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeIntegrationTrustPlaneTargetName = ""
	adoptBootstrapLeftoverMarkerFromDeadProcess()
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("adoption must never consume a live owner's marker: %v", err)
	}
	// pid 2 (kthreadd) is alive on every Linux: adoption must abstain.
	if err := os.WriteFile(markerPath, []byte("2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	adoptBootstrapLeftoverMarkerFromDeadProcess()
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("pid 2 is alive on Linux; adoption must not consume its marker: %v", err)
	}
	// A reaped child pid is provably dead (barring PID reuse).
	child := exec.Command("/bin/true")
	if err := child.Run(); err != nil {
		t.Fatal(err)
	}
	dead := child.Process.Pid
	if err := os.WriteFile(markerPath, []byte(strconv.Itoa(dead)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	adoptBootstrapLeftoverMarkerFromDeadProcess()
	if !bootstrapProcessAlive(dead) {
		if _, statErr := os.Stat(markerPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("a dead owner's marker must be adopted and consumed, stat err: %v", statErr)
		}
	} else {
		t.Skip("pid was reused and is live again; adoption correctly abstained")
	}
}

// TestBootstrapPanickingHelperCallerSubprocess is only meaningful inside the
// dedicated panicking subprocess started by
// TestBootstrapPackageTeardownSurvivesPanickingTest (stage one). It claims a
// bootstrap target and then panics, proving that the durable marker survives
// the Go-unreachable os.Exit that a test panic escalates to.
func TestBootstrapPanickingHelperCallerSubprocess(t *testing.T) {
	if os.Getenv(bootstrapPackageTeardownEnv) != "1" {
		t.Skip("subprocess-only fixture for the package teardown panic path")
	}
	marker := os.Getenv(bootstrapPackageTeardownMarkerEnv)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("TestMain must have created the teardown marker before m.Run: %v", err)
	}
	// Claim the target exactly like the real helper does at the earliest
	// possible moment, so the durable claim (marker + name) exists even if
	// the panic beats everything else.
	runtimeIntegrationTrustPlaneTargetName = "w169-ci-bootstrap-" + bootstrapTargetSuffix()
	defer func() {
		_, statErr := os.Stat(marker)
		fmt.Printf("stage one durable claim present: %v (%v)\n", statErr == nil, statErr)
	}()
	panic("simulated mid-test panic after the helper claimed the bootstrap target")
}

func TestBootstrapLeftoverMarkerLifecycle(t *testing.T) {
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv(bootstrapPackageTeardownMarkerEnv, "")

	marker := setupBootstrapLeftoverMarker()
	if marker != filepath.Join(dir, bootstrapLeftoverMarkerName) {
		t.Fatalf("unexpected marker path %q", marker)
	}
	if err := ensureBootstrapLeftoverMarker(); err != nil {
		t.Fatalf("freshly created marker must satisfy the guard: %v", err)
	}
	// A leftover marker from an interrupted run of THIS package invocation is
	// replaced, never adopted: the new owner is this process.
	if err := os.WriteFile(marker, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := setupBootstrapLeftoverMarker(); got != marker {
		t.Fatalf("setup must replace and return the same marker path, got %q", got)
	}
	if err := ensureBootstrapLeftoverMarker(); err != nil {
		t.Fatalf("after setup the marker must be owned by this process: %v", err)
	}

	// Outside the runtime-integration cell the marker is inert so unit-only
	// `go test ./...` runs never touch a data home they do not provision.
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "0")
	if got := setupBootstrapLeftoverMarker(); got != "" {
		t.Fatalf("non-cell runs must not create a marker: %q", got)
	}
	if err := destroyBootstrapTrustPlane(""); err != nil {
		t.Fatalf("an empty marker is a no-op teardown: %v", err)
	}

	// A process-owned marker with no claimed target (the helper never reached
	// provisioning) is consumed by the package teardown without destroying
	// anything it does not own.
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	marker = filepath.Join(dir, bootstrapLeftoverMarkerName)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := pushPackageTeardownState()
	t.Cleanup(restore)
	runtimeIntegrationTrustPlaneTargetName = ""
	if err := destroyBootstrapTrustPlane(marker); err != nil {
		t.Fatalf("teardown of a never-provisioned claimed run: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the package teardown must consume the marker, stat err: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Package-teardown subprocess support
// ---------------------------------------------------------------------------

const (
	bootstrapPackageTeardownEnv       = "BASEHARBOR_TEST_PACKAGE_TEARDOWN"
	bootstrapPackageTeardownMarkerEnv = "BASEHARBOR_TEST_PACKAGE_TEARDOWN_MARKER"
)

func truncateForLog(output []byte) string {
	const limit = 4000
	text := string(output)
	if len(text) > limit {
		return text[:limit] + "\n... [truncated]"
	}
	return text
}

func readMainTestSupportSource(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(wd, "runtime_integration_test_support_test.go"))
	if err != nil {
		t.Fatalf("read test support source: %v", err)
	}
	return string(data)
}
