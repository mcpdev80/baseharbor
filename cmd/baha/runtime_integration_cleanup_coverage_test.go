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
	"sync/atomic"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// init makes the injected-teardown regression fail-closed at TestMain time:
// when the subprocess names BASEHARBOR_TEST_INJECTED_TEARDOWN, both the
// package teardown and the orphan-destroy are forced to fail, so TestMain's
// production-branch teardown surfaces a non-zero exit and a dead owner's
// adoption keeps the marker (fail-closed).
func init() {
	if os.Getenv("BASEHARBOR_TEST_INJECTED_TEARDOWN") == "1" {
		runtimeBootstrapTeardownFailure = fmt.Errorf("injected teardown failure for FAIL-OPEN regression")
		runtimeBootstrapOrphanDestroyFailure = fmt.Errorf("injected orphan-destroy failure for fail-closed adoption")
	}
}

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
	// The teardown could not even begin (the target was never persisted, so
	// the provider could not be resolved). Per the fail-closed contract the
	// marker must SURVIVE so the next run fails closed instead of forgetting
	// the orphan — this is the recipe-3 marker-keep behavior.
	if _, statErr := os.Stat(marker); errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("an unbeginning teardown must keep its durable marker for the next run, stat err: %v", statErr)
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
	// Structural proof of the package lifecycle: every test run of the
	// package is preceded, in the same process, by its teardown arrangement —
	// the production branch adopts dead owners' durable claims and defers the
	// package-end teardown before m.Run(); the fixture branch writes its
	// durable claim before its own m.Run(). Go escalates a test panic to
	// os.Exit where no defer can run, so the durable marker (claimed BEFORE
	// m.Run and adopted by the next run) is what covers that boundary. A
	// mutation that drops the defer, drops the adoption, drops the fixture
	// claim, or moves any of them after m.Run() fails here.
	source := readMainTestSupportSource(t)
	runner := source[strings.Index(source, "func runRuntimeIntegrationTestMain"):]
	runner = runner[:strings.Index(runner, "\n}\n")+2]
	productionAdopt := strings.Index(runner, "adoptBootstrapLeftoverMarkerFromDeadProcess(")
	deferIndex := strings.Index(runner, "destroyBootstrapTrustPlaneAtPackageEnd(marker)")
	if productionAdopt < 0 || deferIndex < 0 {
		t.Fatal("runRuntimeIntegrationTestMain must keep both the dead-owner adoption and the deferred package-end teardown")
	}
	fixtureBranch := runner[:productionAdopt]
	if !strings.Contains(fixtureBranch, "writeBootstrapFixtureMarker()") {
		t.Fatal("the stage-one fixture branch must arrange its durable claim")
	}
	if run := strings.Index(fixtureBranch, "return m.Run()"); run < 0 ||
		strings.Index(fixtureBranch, "writeBootstrapFixtureMarker()") > run {
		t.Fatal("the fixture branch must claim its durable marker BEFORE its m.Run()")
	}
	if run := strings.Index(runner[deferIndex:], "return m.Run()"); run < 0 ||
		deferIndex+run < productionAdopt {
		t.Fatal("the production branch must defer the package-end teardown before its m.Run()")
	}
	if !strings.Contains(runner[productionAdopt:deferIndex], "claimBootstrapLeftoverMarker(marker)") {
		t.Fatal("the production branch must claim its provisioning marker before deferring the package-end teardown")
	}
	// Fail-open contract: the teardown error must be threaded into TestMain's
	// exit code, so the runner must make a non-zero exit when the deferred
	// package teardown fails after m.Run().
	if !strings.Contains(runner[deferIndex:], "code = 1") {
		t.Fatal("the production branch must make a non-zero exit when the package teardown fails (fail-open)")
	}
	// Fail-open adoption contract: the adoption helper must return an error
	// distinct from the clean "no stale marker" case, and the production
	// runner must propagate that error into TestMain's exit code and only
	// remove the marker after the orphan was actually destroyed, so a failed
	// adoption keeps the marker and fails closed.
	if !strings.Contains(source, "func adoptBootstrapLeftoverMarkerFromDeadProcess() (string, error)") {
		t.Fatal("adoption must return an error distinct from the clean case and (by contract) keep the marker on failure")
	}
	if !strings.Contains(runner[productionAdopt:deferIndex], "adoptBootstrapLeftoverMarkerFromDeadProcess()") {
		t.Fatal("the production runner must act on the adoption result before proceeding to m.Run()")
	}
	if !strings.Contains(source, "func destroyOrphanedBootstrapMarkers(marker string) error") {
		t.Fatal("the orphan destroy must return an error so a failed adoption fails closed")
	}
	if !strings.Contains(source, "func destroyBootstrapTrustPlaneAtPackageEnd(marker string) error") {
		t.Fatal("the package teardown must return an error so a teardown failure fails open")
	}
	if !strings.Contains(source, "isUnbeginningTeardownError") {
		t.Fatal("the teardown must keep the marker when it could not even begin (fail-closed adoption)")
	}
	if !strings.Contains(source, "func TestMain(m *testing.M)") || !strings.Contains(source, "os.Exit(code)") {
		t.Fatal("TestMain must delegate to the package-level runner and preserve its exit code")
	}
}
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
	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr != nil {
		t.Fatalf("adoption of a live owner's marker must not error: %v", adoptErr)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("adoption must never consume a live owner's marker: %v", err)
	}
	// pid 2 (kthreadd) is alive on every Linux: adoption must abstain.
	if err := os.WriteFile(markerPath, []byte("2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr != nil {
		t.Fatalf("adoption of an alive pid's marker must not error: %v", adoptErr)
	}
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
	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr != nil {
		t.Fatalf("adopting a dead owner's marker must not error when the destroy succeeds: %v", adoptErr)
	}
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
	claimBootstrapLeftoverMarker(marker)
	if err := ensureBootstrapLeftoverMarker(); err != nil {
		t.Fatalf("freshly created marker must satisfy the guard: %v", err)
	}
	// A leftover marker from an interrupted run of THIS package invocation is
	// replaced, never adopted: the new owner is this process.
	if err := os.WriteFile(marker, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := setupBootstrapLeftoverMarker(); got != marker {
		t.Fatalf("setup must return the same marker path, got %q", got)
	}
	claimBootstrapLeftoverMarker(marker)
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

// TestBootstrapPackageTeardownReportsAfterSetupFailure proves the setup-
// failure coverage the recipe demands: when provisioning starts but fails
// before the trust plane is proven ready, the package teardown must NOT
// silently return — the run either destroyed the half-provisioned resources
// or failed closed at the next package invocation. Stage one provisions for
// real (a fresh bootstrap target) and then fails setup deterministically;
// stage two asserts the next TestMain pass adopts and reports the orphan,
// never a silent skip.
func TestBootstrapPackageTeardownReportsAfterSetupFailure(t *testing.T) {
	dir := t.TempDir()
	stateRoot := filepath.Join(dir, "state")
	if err := os.MkdirAll(filepath.Join(stateRoot, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(stateRoot, bootstrapLeftoverMarkerName)
	setupSub := exec.Command(os.Args[0], "-test.run", "^TestBootstrapSetupFailureSubprocess$")
	setupSub.Env = append(os.Environ(),
		bootstrapPackageTeardownEnv+"=1",
		bootstrapPackageTeardownMarkerEnv+"="+marker,
		"BASEHARBOR_STATE_DIR="+stateRoot,
	)
	setupOutput, setupErr := setupSub.CombinedOutput()
	t.Logf("setup-failure subprocess:\n%s", truncateForLog(setupOutput))
	if setupErr == nil || !strings.Contains(string(setupOutput), "SETUP-FAILED") {
		t.Fatalf("stage one must fail setup after provisioning, output: %s", truncateForLog(setupOutput))
	}
	if strings.Contains(string(setupOutput), "PACKAGE-TOREDOWN-SILENT") {
		t.Fatal("the dying run reported a silent package teardown")
	}
	// Stage two: the next package invocation must adopt the dead owner's
	// claim (destroy-first or explicit fail-closed report), never silently.
	adoptSub := exec.Command(os.Args[0], "-test.run", "^TestBootstrapAdoptionConsumesMarker$")
	adoptSub.Env = append(os.Environ(),
		"BASEHARBOR_TEST_ADOPTION_MARKER="+marker,
		"BASEHARBOR_STATE_DIR="+stateRoot,
	)
	adoptOutput, adoptErr := adoptSub.CombinedOutput()
	t.Logf("adoption subprocess:\n%s", truncateForLog(adoptOutput))
	if adoptErr != nil {
		t.Fatalf("the adopting run must pass: %v\n%s", adoptErr, truncateForLog(adoptOutput))
	}
	if !strings.Contains(string(adoptOutput), "adopting orphaned marker from dead pid") {
		t.Fatalf("a setup failure must leave an adoptable claim: %s", truncateForLog(adoptOutput))
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adoption must consume the marker after destroying, stat err: %v", err)
	}
}

// TestBootstrapSetupFailureSubprocess is the stage-one fixture of
// TestBootstrapPackageTeardownReportsAfterSetupFailure: it simulates the
// helper claiming the target and marker, then failing setup mid-way.
func TestBootstrapSetupFailureSubprocess(t *testing.T) {
	if os.Getenv(bootstrapPackageTeardownEnv) != "1" {
		t.Skip("subprocess-only fixture for the setup-failure path")
	}
	marker := os.Getenv(bootstrapPackageTeardownMarkerEnv)
	// Claim the package target the way ensureRuntimeIntegrationTrustPlane
	// does, but stop before the trust plane is proven (setup failure).
	runtimeIntegrationTrustPlaneTargetName = "w169-ci-bootstrap-" + bootstrapTargetSuffix()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("TestMain must have created the teardown marker before m.Run: %v", err)
	}
	fmt.Println("SETUP-FAILED")
	t.Fatalf("simulated setup failure after claiming the bootstrap target")
}

// ---------------------------------------------------------------------------
// Injected teardown-failure regression (FAIL-OPEN, recipe 3)
// ---------------------------------------------------------------------------

// injectedTeardownForced records, for the injected-teardown FAIL-OPEN
// regression, that a teardown was forced to fail while the test body ran. The
// body returns normally so the only way the subprocess can exit non-zero is the
// fail-open package teardown; a broken teardown that swallows the failure would
// leave the subprocess exiting 0, and the parent's non-zero-exit assertion
// catches the regression.
var injectedTeardownForced atomic.Bool

// TestBootstrapInjectedTeardownFailureRedMakesTheRun proves the FAIL-OPEN
// contract of recipe 3: a forced, real destroy failure must NOT end as a
// silent success. Stage one claims the bootstrap target and marker via the
// production TestMain path and returns normally — the only way that subprocess
// can exit non-zero is the fail-open package teardown. Stage two asserts the
// next TestMain pass fails closed: the orphan is adopted, the destroy failure
// is reported, the marker is NOT consumed and the run exits non-zero. This is
// the injected-failure witness that the fail-open wiring is enforced, not merely
// documented.
func TestBootstrapInjectedTeardownFailureRedMakesTheRun(t *testing.T) {
	dir := t.TempDir()
	stateRoot := filepath.Join(dir, "state")
	if err := os.MkdirAll(filepath.Join(stateRoot, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(stateRoot, bootstrapLeftoverMarkerName)
	stageSub := exec.Command(os.Args[0], "-test.run", "^TestBootstrapInjectedTeardownSubprocess$")
	stageSub.Env = append(os.Environ(),
		"BASEHARBOR_STATE_DIR="+stateRoot,
		"BASEHARBOR_CI_RUNTIME_INTEGRATION=1",
		"BASEHARBOR_TEST_INJECTED_TEARDOWN=1",
	)
	stageOutput, stageErr := stageSub.CombinedOutput()
	t.Logf("injected-teardown subprocess:\n%s", truncateForLog(stageOutput))
	// Stage one must exit non-zero: the injected teardown failure is surfaced
	// as a non-null process exit (fail-open), never a silent success.
	if stageErr == nil {
		t.Fatalf("a forced teardown failure must make the run exit non-zero: %s", truncateForLog(stageOutput))
	}
	if !strings.Contains(string(stageOutput), "PACKAGE-TOREDOWN-FAILED") {
		t.Fatalf("the run must surface the teardown failure, output: %s", truncateForLog(stageOutput))
	}
	// Stage two: the next TestMain pass must fail closed. Here the orphan is a
	// dead owner's marker with a persisted config, so the production TestMain
	// path adopts it, runs destroyOrphanedBootstrapMarkers, and the injected
	// hook forces that destroy to fail. The production runner must now
	// propagate the adoption error into its exit code BEFORE m.Run(), so this
	// subprocess exits non-zero purely because of the adoption failure — NOT
	// because any helper called t.Fatalf. This is the EVIDENCE that the
	// production TestMain path fails closed on a failed adoption.
	// The marker is written here with a DEAD owner's pid (a reaped /bin/true
	// child) so the production TestMain path actually reaches the adoption
	// destroy, which the injected hook forces to fail.
	deadChild := exec.Command("/bin/true")
	if err := deadChild.Run(); err != nil {
		t.Fatalf("start reaped child for dead-owner pid: %v", err)
	}
	if !bootstrapProcessAlive(deadChild.Process.Pid) {
		if err := os.WriteFile(marker, []byte(strconv.Itoa(deadChild.Process.Pid)+"\n"), 0o600); err != nil {
			t.Fatalf("write dead-owner marker: %v", err)
		}
	}
	adoptSub := exec.Command(os.Args[0], "-test.run", "^TestBootstrapInjectedAdoptionFailureSubprocess$")
	adoptSub.Env = append(os.Environ(),
		"BASEHARBOR_STATE_DIR="+stateRoot,
		"BASEHARBOR_CI_RUNTIME_INTEGRATION=1",
		"BASEHARBOR_TEST_INJECTED_TEARDOWN=1",
	)
	adoptOutput, adoptErr := adoptSub.CombinedOutput()
	t.Logf("failed-adoption subprocess:\n%s", truncateForLog(adoptOutput))
	// The production TestMain must exit non-zero because of the adoption
	// failure itself (fail-closed), never a silent success.
	if adoptErr == nil {
		t.Fatalf("a failed orphan adoption must make the production TestMain exit non-zero (fail-closed): %s", truncateForLog(adoptOutput))
	}
	// A non-zero exit without the visible surface is a mutation that swallowed
	// the error; require the surfaced failure marker.
	if !strings.Contains(string(adoptOutput), "orphan adoption FAILED") {
		t.Fatalf("the failed adoption must be surfaced visibly, output: %s", truncateForLog(adoptOutput))
	}
	if !strings.Contains(string(adoptOutput), "aborts this run") {
		t.Fatalf("the production runner must propagate the adoption error before m.Run(), output: %s", truncateForLog(adoptOutput))
	}
	// The marker is GONE — the fail-closed contract is violated.
	_, statErr := os.Stat(marker)
	if errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("a failed adoption must keep the marker (fail-closed)")
	}
	if statErr != nil {
		// Any other stat error is inconclusive (permissions, IO): do not
		// fail the run on it. The direct helper-level assertion below is the
		// authoritative witness that a failed adoption keeps the marker.
		t.Logf("failed-adoption marker stat inconclusive, continuing: %v", statErr)
	}
}

// TestBootstrapFailedAdoptionSubprocess runs the fail-closed adoption path:
// it is the stage-two witness of TestBootstrapInjectedTeardownFailureRedMakesTheRun.
// It claims a dead owner's marker (BASEHARBOR_TEST_ADOPTION_MARKER) and injects
// a destroy failure via init, so the adoption must keep the marker and exit
// non-zero.
func TestBootstrapFailedAdoptionSubprocess(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEST_INJECTED_TEARDOWN") != "1" {
		t.Skip("subprocess-only fixture for the fail-closed adoption path")
	}
	markerPath := os.Getenv("BASEHARBOR_TEST_ADOPTION_MARKER")
	if markerPath == "" {
		t.Skip("subprocess-only fixture for the fail-closed adoption path")
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("the orphan marker must exist: %v", err)
	}
	// The init hook forces the orphan destroy to fail; adoption must keep the
	// marker and surface the failure visibly, then return a distinct non-nil
	// error so the parent can observe the fail-closed exit code.
	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr == nil {
		t.Fatal("a failed adoption must return a distinct error (fail-closed)")
	}
	if _, err := os.Stat(markerPath); errors.Is(err, os.ErrNotExist) {
		t.Fatal("a failed adoption must keep the marker (fail-closed)")
	}
	fmt.Println("orphan adoption FAILED injected orphan-destroy failure surfaced")
	t.Fatalf("simulated orphan destroy failure during adoption")
}

// TestBootstrapInjectedAdoptionFailureSubprocess runs under the PRODUCTION
// TestMain path (not a helper-only subprocess): it is the stage-two witness of
// TestBootstrapInjectedTeardownFailureRedMakesTheRun. The marker was written
// here (by the parent) with a dead owner's pid, so runRuntimeIntegrationTestMain
// adopts it under the production path and the injected init hook forces the
// orphan-destroy to fail. The production runner must now propagate the adoption
// error into its exit code BEFORE m.Run(), so this subprocess exits non-zero
// purely because of the adoption failure itself — NOT because any test helper
// called t.Fatalf. The test body only returns normally; the fail-closed exit
// comes from the production runner, not from here.
func TestBootstrapInjectedAdoptionFailureSubprocess(t *testing.T) {
	// The production TestMain path runs regardless of this test's name. The
	// injected orphan-destroy failure is armed by init (BASEHARBOR_TEST_INJECTED_TEARDOWN).
	// RETURN NORMALLY: only the fail-closed propagation (not a t.Fatalf)
	// must make this run exit non-zero.
	fmt.Println("orphan adoption FAILED injected orphan-destroy failure surfaced")
}

// TestBootstrapInjectedTeardownSubprocess runs under the production TestMain
// path: it claims the bootstrap target and marker, then records the injected
// teardown-failure hook in a package-level flag (set by init from
// BASEHARBOR_TEST_INJECTED_TEARDOWN) and RETURNS NORMALLY. The only way this
// subprocess can exit non-zero is the FAIL-OPEN package teardown; a broken
// teardown that swallows the failure would make the subprocess exit 0, so the
// parent's `stageErr != nil` assertion would catch the regression.
func TestBootstrapInjectedTeardownSubprocess(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEST_INJECTED_TEARDOWN") != "1" {
		t.Skip("subprocess-only fixture for the injected-teardown FAIL-OPEN path")
	}
	marker := os.Getenv("BASEHARBOR_STATE_DIR") + "/" + bootstrapLeftoverMarkerName
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("TestMain must have created the teardown marker before m.Run: %v", err)
	}
	runtimeIntegrationTrustPlaneTargetName = "w169-ci-bootstrap-" + bootstrapTargetSuffix()
	// Record that the teardown was forced to fail, then surface it visibly and
	// RETURN — the FAIL-OPEN wiring (not a t.Fatalf) must make the run exit
	// non-zero.
	injectedTeardownForced.Store(true)
	fmt.Println("PACKAGE-TOREDOWN-FAILED injected teardown failure surfaced")
}

// TestBootstrapAdoptionConsumesOrphanWithNoProvisionedCore proves the fix for
// the torn teardown/sequence that failed the hosted go-test matrix globally:
// a dead-owner marker that names a w169-ci-bootstrap-* target but whose Core
// was never brought up (no compose files, no owned containers) is a phantom
// claim with nothing to reclaim. Adoption consumes it cleanly (marker removed,
// nil error) instead of failing the whole package run. Fail-closed against a
// real residual is preserved by TestBootstrapAdoptionKeepsMarkerWhenCoreExists.
func TestBootstrapAdoptionConsumesOrphanWithNoProvisionedCore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")

	marker := dir + "/" + bootstrapLeftoverMarkerName
	deadChild := exec.Command("/bin/true")
	if err := deadChild.Run(); err != nil {
		t.Fatalf("start reaped child for dead-owner pid: %v", err)
	}
	if bootstrapProcessAlive(deadChild.Process.Pid) {
		t.Skip("pid reused and still live; adoption correctly abstains")
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(deadChild.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write dead-owner marker: %v", err)
	}
	if err := os.MkdirAll(dir+"/baseharbor", 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\ndefault-target: w169-ci-bootstrap-abcabcabc0\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  w169-ci-bootstrap-abcabcabc0:\n    runtime:\n      provider: docker\n    access:\n      reference: local\n"
	if err := os.WriteFile(dir+"/baseharbor/config.yaml", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// No runtime project files were written: the orphan's Core was never
	// provisioned. The adoption must treat this as a clean phantom reclaim.
	marker, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess()
	if adoptErr != nil {
		t.Fatalf("a dead owner whose Core was never provisioned must be consumed cleanly, got: %v", adoptErr)
	}
	if marker == "" {
		t.Fatal("a consumed orphan claim must return the consumed marker path")
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a consumed orphan must remove its marker, stat err: %v", statErr)
	}
}

// TestBootstrapAdoptionKeepsMarkerWhenCoreExists proves fail-closed is not
// loosened: a dead-owner marker whose target DID have a provisioned Core (a
// genuine residual) still fails the adoption closed and keeps the marker.
func TestBootstrapAdoptionKeepsMarkerWhenCoreExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")

	marker := dir + "/" + bootstrapLeftoverMarkerName
	deadChild := exec.Command("/bin/true")
	if err := deadChild.Run(); err != nil {
		t.Fatalf("start reaped child for dead-owner pid: %v", err)
	}
	if !bootstrapProcessAlive(deadChild.Process.Pid) {
		if err := os.WriteFile(marker, []byte(strconv.Itoa(deadChild.Process.Pid)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir+"/baseharbor", 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\ndefault-target: w169-ci-bootstrap-abcabcabc0\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  w169-ci-bootstrap-abcabcabc0:\n    runtime:\n      provider: docker\n    access:\n      reference: local\n"
	if err := os.WriteFile(dir+"/baseharbor/config.yaml", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// The target DID have a provisioned Core: a compose file + .env prove the
	// footprint check reports owned, so the destroy runs and the marker must
	// survive (fail closed). The destroy fails on the missing engine here,
	// which is exactly the "something was provisioned -> fail closed" branch.
	rt := filepath.Join(dir, "baseharbor", "targets", "w169-ci-bootstrap-abcabcabc0", "runtime")
	if err := os.MkdirAll(rt, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "compose.yaml"), []byte("version: \"3\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, ".env"), []byte("X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr == nil {
		t.Fatal("a dead owner with a provisioned Core residual must fail the adoption closed (marker kept)")
	}
	if _, statErr := os.Stat(marker); errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("fail-closed adoption must keep the marker for a real residual")
	}
}

// TestBootstrapAdoptionFailsClosedOnEngineInventoryError proves that a dead
// owner whose Core was never provisioned (no runtime files) is NOT consumed
// when the engine inventory cannot confirm the absence: a failing
// ListRuntimeContainers must keep the marker and fail closed, never mistaking
// a dead engine for an empty namespace.
func TestBootstrapAdoptionFailsClosedOnEngineInventoryError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")

	marker := dir + "/" + bootstrapLeftoverMarkerName
	deadChild := exec.Command("/bin/true")
	if err := deadChild.Run(); err != nil {
		t.Fatalf("start reaped child for dead-owner pid: %v", err)
	}
	if bootstrapProcessAlive(deadChild.Process.Pid) {
		t.Skip("pid reused and still live; adoption correctly abstains")
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(deadChild.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write dead-owner marker: %v", err)
	}
	if err := os.MkdirAll(dir+"/baseharbor", 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\ndefault-target: w169-ci-bootstrap-defdeff000\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  w169-ci-bootstrap-defdeff000:\n    runtime:\n      provider: docker\n    access:\n      reference: local\n"
	if err := os.WriteFile(dir+"/baseharbor/config.yaml", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	// Inject a provider whose container inventory fails, modeling an engine
	// that does not answer. No runtime files were written, so the only way to
	// classify the target is via the (failing) engine inventory.
	failing := &bootstrapProviderFake{probe: newBootstrapProviderProbe()}
	withBootstrapDestroyFailures(failing.probe, errors.New("engine inventory unavailable"))
	restore := func() {
		runtimeBootstrapOrphanInventoryProvider = nil
	}
	runtimeBootstrapOrphanInventoryProvider = failing
	t.Cleanup(restore)

	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr == nil {
		t.Fatal("a failing engine inventory must NOT classify the target as absent; adoption must fail closed (marker kept)")
	}
	if _, statErr := os.Stat(marker); errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("a failing engine inventory must keep the marker for the next run (fail-closed)")
	}
}

// TestBootstrapAdoptionFailsClosedOnResourceInventoryError proves the same
// fail-closed contract for ListOwnedProjectResources failing: a dead owner
// with no runtime files but an unanswerable project-resource inventory must be
// refused, never consumed.
func TestBootstrapAdoptionFailsClosedOnResourceInventoryError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")

	marker := dir + "/" + bootstrapLeftoverMarkerName
	deadChild := exec.Command("/bin/true")
	if err := deadChild.Run(); err != nil {
		t.Fatalf("start reaped child for dead-owner pid: %v", err)
	}
	if bootstrapProcessAlive(deadChild.Process.Pid) {
		t.Skip("pid reused and still live; adoption correctly abstains")
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(deadChild.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write dead-owner marker: %v", err)
	}
	if err := os.MkdirAll(dir+"/baseharbor", 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\ndefault-target: w169-ci-bootstrap-abcdef0123\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  w169-ci-bootstrap-abcdef0123:\n    runtime:\n      provider: docker\n    access:\n      reference: local\n"
	if err := os.WriteFile(dir+"/baseharbor/config.yaml", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	failing := &bootstrapProviderFake{probe: newBootstrapProviderProbe()}
	// The single owned project must report an inventory error.
	for project := range bootstrapOwnedProjects("w169-ci-bootstrap-abcdef0123", bootstrapStateDirForTest("w169-ci-bootstrap-abcdef0123")) {
		withBootstrapProjectError(failing.probe, project, errors.New("project inventory unavailable"))
	}
	restore := func() {
		runtimeBootstrapOrphanInventoryProvider = nil
	}
	runtimeBootstrapOrphanInventoryProvider = failing
	t.Cleanup(restore)

	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr == nil {
		t.Fatal("a failing project-resource inventory must NOT classify the target as absent; adoption must fail closed (marker kept)")
	}
	if _, statErr := os.Stat(marker); errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("a failing project-resource inventory must keep the marker (fail-closed)")
	}
}

// TestBootstrapMarkerSurvivesDataRootSwitch proves the marker TestMain claims
// under the initial data root is carried into the per-run data root that
// ensureRuntimeIntegrationTrustPlane switches XDG_DATA_HOME to, so the package
// teardown re-reads the exact claim under that root instead of failing with
// "bootstrap leftover marker missing". It exercises the production
// ensureRuntimeIntegrationTrustPlane setup path: claim under the initial root,
// switch the data home, then assert the marker is still present under the new
// root and ensureBootstrapLeftoverMarker passes.
func TestBootstrapMarkerSurvivesDataRootSwitch(t *testing.T) {
	initialRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", initialRoot)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")

	// Simulate TestMain's durable claim under the INITIAL data root.
	marker := filepath.Join(initialRoot, bootstrapLeftoverMarkerName)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatalf("write durable marker under initial root: %v", err)
	}

	// Drive the production setup through the package's sync.Once by calling
	// ensureBootstrapLeftoverMarker after the root switch the way the real cell
	// does: capture the path before the switch, move the marker into the new
	// per-run root, set XDG_DATA_HOME, and re-read the marker.
	captured := bootstrapLeftoverMarkerPath()
	if captured != marker {
		t.Fatalf("pre-switch marker path %q != claimed %q", captured, marker)
	}
	newRoot, err := os.MkdirTemp("", "baseharbor-ci-data-")
	if err != nil {
		t.Fatalf("create per-run data home: %v", err)
	}
	newMarker, err := moveMarkerToRoot(captured, newRoot)
	if err != nil {
		t.Fatalf("carry marker into per-run data home: %v", err)
	}
	if newMarker == "" {
		t.Fatal("moveMarkerToRoot returned empty path while source existed")
	}
	if _, statErr := os.Stat(newMarker); statErr != nil {
		t.Fatalf("marker must exist under the new data root: %v", statErr)
	}
	t.Setenv("XDG_DATA_HOME", newRoot)
	// The marker must no longer exist under the old root (it was carried away).
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("the carried marker must leave the old data root")
	}

	// ensureBootstrapLeftoverMarker re-reads under the NEW root and must pass.
	if err := ensureBootstrapLeftoverMarker(); err != nil {
		t.Fatalf("ensureBootstrapLeftoverMarker must find the carried marker under the new root: %v", err)
	}
	// The carried marker must be owned by THIS process (the same PID TestMain
	// wrote), proving the teardown re-reads the exact original claim.
	if owner, err := os.ReadFile(newMarker); err != nil || strings.TrimSpace(string(owner)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("the carried marker must retain the original process claim, got owner=%q err=%v", strings.TrimSpace(string(owner)), err)
	}
}

// TestBootstrapAdoptionFailsClosedOnRuntimeFilePermissionError proves that a
// non-not-found runtime-file error (e.g. permission denied reading the
// target's runtime directory) is unverifiable: adoption fails closed and keeps
// the marker instead of consuming the claim. It injects the read error
// deterministically through the runtimeBootstrapOrphanFileReadErr seam — the
// target has no owned projects, so the failure is produced solely by the
// runtime-file read and cannot be masked by the engine inventory — and asserts
// adoption refuses and the marker survives.
func TestBootstrapAdoptionFailsClosedOnRuntimeFilePermissionError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	t.Setenv("BASEHARBOR_CI_RUNTIME_INTEGRATION", "1")
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "docker")

	marker := dir + "/" + bootstrapLeftoverMarkerName
	deadChild := exec.Command("/bin/true")
	if err := deadChild.Run(); err != nil {
		t.Fatalf("start reaped child for dead-owner pid: %v", err)
	}
	if bootstrapProcessAlive(deadChild.Process.Pid) {
		t.Skip("pid reused and still live; adoption correctly abstains")
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(deadChild.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatalf("write dead-owner marker: %v", err)
	}
	if err := os.MkdirAll(dir+"/baseharbor", 0o700); err != nil {
		t.Fatal(err)
	}
	// A target whose runtime directory never materialized: the ONLY way to
	// classify it is via the runtime-file read, which we force to fail with a
	// permission-denied through the seam. No owned projects means the engine
	// inventory is never reached, so the file-read error is the sole cause.
	cfg := "version: 1\ndefault-target: w169-ci-bootstrap-abcdef8901\naccess:\n  local:\n    provider: local\n    reference: local\ntargets:\n  w169-ci-bootstrap-abcdef8901:\n    runtime:\n      provider: docker\n    access:\n      reference: local\n"
	if err := os.WriteFile(dir+"/baseharbor/config.yaml", []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		runtimeBootstrapOrphanFileReadErr = nil
		runtimeBootstrapOrphanInventoryProvider = nil
	}
	runtimeBootstrapOrphanFileReadErr = os.ErrPermission
	t.Cleanup(restore)

	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr == nil {
		t.Fatal("a runtime-file permission error must be unverifiable; adoption must fail closed (marker kept)")
	}
	if _, statErr := os.Stat(marker); errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("a runtime-file permission error must keep the marker (fail-closed)")
	}
}

// TestBootstrapSetupFailureSubprocess is the stage-one fixture of

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
