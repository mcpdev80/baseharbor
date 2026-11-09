package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	identityprovider "github.com/mcpdev80/baseharbor/internal/identityprovider"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var (
	runtimeIntegrationTrustPlaneOnce sync.Once
	runtimeIntegrationTrustPlaneErr  error
)

// bootstrapLeftoverMarkerName is the package-scoped state file the setup
// creates FIRST, before any operation that can materialize runtime resources.
// Its presence is the fail-safe contract between setup and the package-level
// teardown: while the marker exists, provisioning has begun but teardown is
// not yet guaranteed, so TestMain destroys the bootstrap trust plane on BOTH
// the success and the failure path. The teardown removes the marker only once
// the destruction itself has run to completion, so the bootstrap Core can
// never become a residual bh-w169-ci-bootstrap-* container/volume/network
// after the package. The owning target + runtime provider live in this file
// because os.Setenv is not rolled back at test end (unlike t.Setenv), so the
// teardown may run after every test that captured the pair has completed.
const bootstrapLeftoverMarkerName = "baseharbor-bootstrap-cleanup-pending"

// Single deterministic teardown of the shared bootstrap trust-plane. Its
// lifecycle owner is the package, not a *testing.T: TestMain runs it exactly
// once after ALL tests of the package finished (m.Run returned) or panicked
// (runtime.Goexit reached the deferred teardown). A t.Cleanup on the first
// caller would destroy the singleton while later helper callers still expect
// it, and the sync.Once would prevent recreation. This helper owns no product
// destroy path — it only removes the resources the test suite itself
// materialized.
var (
	runtimeIntegrationTrustPlaneTarget     deployment.ResolvedTarget
	runtimeIntegrationTrustPlaneProvider   bhruntime.RuntimeProvider
	runtimeIntegrationTrustPlaneTargetName string
	// runtimeIntegrationTrustPlaneDataRoot records the data root the shared
	// trust plane now owns. TestMain claims the durable marker under the
	// initial data root; when ensureRuntimeIntegrationTrustPlane moved that
	// claim into a per-run root (see below), this holds the new location so
	// ensureBootstrapLeftoverMarker re-reads the marker from exactly the root
	// that owns it. Empty means "no root switch happened", i.e. the marker
	// lives under the initial data root as TestMain claimed it.
	runtimeIntegrationTrustPlaneDataRoot string
)

// TestMain gives the shared bootstrap trust plane a teardown whose lifetime is
// exactly the package lifetime. Go aborts the whole test binary on a test
// panic (tRunner escalates to os.Exit), so no Go-level defer can observe that
// path: the durable marker file is the claim that survives process death, and
// the next run of the same package invocation adopts and destroys whatever
// the dead process left behind, fail-closed and before any new provisioning.
func TestMain(m *testing.M) {
	// A test panic aborts the process inside tRunner (runtime.Goexit
	// escalates to os.Exit of the binary), so no Go-level defer in the
	// panicking run observes it: the durable marker file is the claim that
	// survives process death, and the NEXT package invocation adopts and
	// destroys whatever the dead process left behind before provisioning
	// anything new (adoptBootstrapLeftoverMarkerFromDeadProcess).
	code := runRuntimeIntegrationTestMain(m)
	os.Exit(code)
}

func runRuntimeIntegrationTestMain(m *testing.M) (code int) {
	// Both branches below arrange their teardown claim BEFORE m.Run(): the
	// fixture branch writes the stage-one marker first, the production
	// branch adopts dead owners' claims and creates its own.
	if os.Getenv(bootstrapPackageTeardownEnv) == "1" && os.Getenv(bootstrapPackageTeardownMarkerEnv) != "" {
		// Stage-one subprocess of the package-lifecycle regression: create
		// the fixture's durable claim before m.Run, exactly like the cell
		// path does; the panicking test body proves it survives.
		if err := writeBootstrapFixtureMarker(); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap fixture marker: %v\n", err)
			return 1
		}
		return m.Run()
	}
	if _, adoptErr := adoptBootstrapLeftoverMarkerFromDeadProcess(); adoptErr != nil {
		// A failed orphan adoption is a real failure: the orphan could not
		// be destroyed, so the durable claim survives and the next run must
		// fail closed. Propagate it here — before m.Run() and before any new
		// provisioning — so the production TestMain exit code is non-zero.
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane teardown: failed adoption aborts this run (marker kept): %v\n", adoptErr)
		return 1
	}
	marker := setupBootstrapLeftoverMarker()
	if marker != "" {
		// Claim the durable marker BEFORE any test can provision, so a
		// mid-setup failure or panic is covered by the package-end teardown
		// (normal return) or by the next run's adoption (os.Exit path).
		claimBootstrapLeftoverMarker(marker)
		// The deferred teardown runs AFTER m.Run() and AFTER code is captured,
		// but it writes the failure into the named return `code`, so TestMain's
		// os.Exit(code) is non-zero when a real teardown failed. The teardown is
		// FAIL-OPEN: a teardown failure is never a silent success.
		defer func() {
			if err := destroyBootstrapTrustPlaneAtPackageEnd(marker); err != nil {
				code = 1
			}
		}()
	}
	return m.Run()
}

// bootstrapLeftoverMarkerPath returns the package-scoped marker file inside
// the configured BaseHarbor data root (BASEHARBOR_STATE_DIR overrides
// XDG_DATA_HOME, mirroring the deployment DataRoot contract), or the empty
// string when no data home is configured (pure unit runs).
// writeBootstrapFixtureMarker claims the fixture's durable marker file for
// this process (stage-one subprocess of the package-teardown regression).
func writeBootstrapFixtureMarker() error {
	return os.WriteFile(os.Getenv(bootstrapPackageTeardownMarkerEnv),
		[]byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
}

func bootstrapLeftoverMarkerPath() string {
	if root := strings.TrimSpace(os.Getenv("BASEHARBOR_STATE_DIR")); root != "" {
		return filepath.Join(root, bootstrapLeftoverMarkerName)
	}
	if root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); root != "" {
		return filepath.Join(root, bootstrapLeftoverMarkerName)
	}
	// Fall back to the deployment DataRoot contract: on a runner (or plain
	// invocation) where neither BASEHARBOR_STATE_DIR nor XDG_DATA_HOME is set,
	// the durable marker still lives beside the runtime state so TestMain and
	// ensureRuntimeIntegrationTrustPlane agree on one root. Without this the
	// marker is never created and the teardown fails with
	// "bootstrap leftover marker missing".
	if root, err := deployment.DataRoot(); err == nil && root != "" {
		return filepath.Join(root, bootstrapLeftoverMarkerName)
	}
	return ""
}

// moveMarkerToRoot relocates the durable provisioning marker from the initial
// data root into the per-run data root the trust plane owns, so the teardown
// re-reads the exact claim TestMain created. It returns the new marker path on
// success, the empty string when nothing was moved (the source does not exist),
// and an error when the copy succeeds but the source cannot be removed.
func moveMarkerToRoot(src, dstRoot string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dstRoot, filepath.Base(src))
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Remove(src); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return dst, nil
}

// setupBootstrapLeftoverMarker creates the provisioning marker before any
// helper call can materialize resources. TestMain calls it exactly once per
// process. Outside the CI runtime-integration cell it only activates when the
// package-lifecycle regression fixture names its marker explicitly (see
// armBootstrapPackageTeardownWitness); every ordinary unit run stays inert
// and touches no data home it did not provision. Markers from a previous,
// interrupted run of the same package invocation are replaced, never adopted:
// a leftover marker from a mid-setup failure makes the package teardown
// destroy the half-provisioned bootstrap trust plane fail-closed.
func setupBootstrapLeftoverMarker() string {
	cell := os.Getenv("BASEHARBOR_CI_RUNTIME_INTEGRATION") == "1"
	if !cell {
		return ""
	}
	marker := bootstrapLeftoverMarkerPath()
	if marker == "" {
		return ""
	}
	return marker
}

// claimBootstrapLeftoverMarker writes this process's durable provisioning
// claim; TestMain calls it before the package test run starts (the cell
// marker path or the fixture marker named by the regression subprocess).
func claimBootstrapLeftoverMarker(marker string) {
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap leftover marker setup failed: %v\n", err)
	}
}

// destroyBootstrapTrustPlaneAtPackageEnd is the TestMain-level teardown used
// when no *testing.T context is available. It reports through stderr because
// it runs after (or without) any test, and it FAILS OPEN BY CONTRACT: the
// returned error is the package teardown's failure, which the caller threads
// into TestMain's exit code via packageTeardownErr so a real teardown failure
// can never end as a silent success. The marker is removed only after the
// destruction ran (see destroyBootstrapTrustPlane); a failure before the
// removal keeps it for the next run's fail-closed adoption.
func destroyBootstrapTrustPlaneAtPackageEnd(marker string) error {
	err := destroyBootstrapTrustPlane(marker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane package teardown failed: %v\n", err)
	}
	return err
}

// bootstrapTeardownClaimed reports whether this process claimed a bootstrap
// target (the fail-safe contract behind destroyBootstrapTrustPlane): either a
// provider was captured or a target name was recorded before provisioning.
func bootstrapTeardownClaimed() bool {
	return runtimeIntegrationTrustPlaneProvider != nil || runtimeIntegrationTrustPlaneTargetName != ""
}

// runtimeBootstrapAdoptionHook is armed by the two-stage subprocess fixture
// only, to prove TestMain consumed the dead owner's claim before m.Run.
// It is nil in every normal run and cannot change teardown semantics.
var runtimeBootstrapAdoptionHook func()

// runtimeBootstrapTeardownObserver lets the package-coverage fixture observe
// the resolved state root of a teardown pass. It is nil in every real run and
// cannot change teardown semantics.
var runtimeBootstrapTeardownObserver func(targetName, dataDir string)

// runtimeBootstrapWatcherObserver lets the package-coverage fixtures see the
// namespace list a watcher pass inspects. It is nil in every real run.
var runtimeBootstrapWatcherObserver func(projects []string)

// runtimeBootstrapTeardownFailure lets the injected-failure regression force
// destroyRuntimeIntegrationTrustPlane to fail. It is nil in every real run and
// cannot change teardown semantics; when set, the teardown returns the wrapped
// error so the FAIL-OPEN wiring must make the run exit non-zero and keep the
// marker for the next run's fail-closed adoption.
var runtimeBootstrapTeardownFailure error

// runtimeBootstrapOrphanDestroyFailure lets the injected-failure regression
// force destroyOrphanedBootstrapMarkers to fail during adoption. It is nil in
// every real run; when set, adoption keeps the marker and fails closed.
var runtimeBootstrapOrphanDestroyFailure error

// runtimeBootstrapOrphanInventoryProvider lets the injected-failure regression
// supply a RuntimeProvider for the dead-owner footprint inventory. It is nil in
// every real run; when set, adoptBootstrapLeftoverMarkerFromDeadProcess derives
// the provider from it (over the real detected one) so a failing engine
// inventory can be exercised for real. When the override returns a nil
// provider, adoption must fail closed rather than classify the target as absent.
var runtimeBootstrapOrphanInventoryProvider bhruntime.RuntimeProvider

// runtimeBootstrapOrphanFileReadErr lets the injected-failure regression force
// the runtime-project-file read (ExistingFilesForProject) inside the dead-owner
// footprint inventory to return a synthetic non-not-found error (e.g. a
// permission-denied), independent of any real filesystem state. It is nil in
// every real run; when set, orphanBootstrapTargetHadFootprint returns that
// error so adoption fails closed instead of mistaking the unreadable runtime
// directory for a never-provisioned phantom. It is exercised exclusively by
// TestBootstrapAdoptionFailsClosedOnRuntimeFilePermissionError.
var runtimeBootstrapOrphanFileReadErr error

// adoptBootstrapLeftoverMarkerFromDeadProcess is the fail-closed recovery for
// the one path no Go defer can observe: Go aborts the whole test binary on a
// test panic (tRunner escalates to os.Exit), so a previous run of this same
// package invocation may have died with claimed, un-destroyed resources. A
// marker owned by a dead PID is adopted, and adoption destroys the orphan
// FIRST and consumes the claim only after the destruction ran (success or
// reported failure) — an interrupted adoption can never fool a later run
// into believing the orphan was already cleaned up. A live owner's marker is
// never touched, so concurrent package invocations stay isolated. It FAILS
// OPEN BY CONTRACT: the returned error is non-nil when the orphan could not
// be destroyed, the marker is NOT removed and a visible error is surfaced so
// the next package run fails closed instead of forgetting the claim.
// adoptBootstrapLeftoverMarkerFromDeadProcess adopts and destroys a dead
// owner's orphaned marker. It returns:
//   - nil, nil           when there is simply no stale marker (clean, proceed)
//   - "", nil            when a live owner's marker was left untouched
//   - "", err            when the orphan destroy FAILED: the marker is kept and
//     the error is surfaced so the caller fails closed.
func adoptBootstrapLeftoverMarkerFromDeadProcess() (string, error) {
	marker := bootstrapLeftoverMarkerPath()
	if marker == "" {
		return "", nil
	}
	owner, err := os.ReadFile(marker)
	if err != nil {
		return "", nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(owner)))
	if err != nil || pid == os.Getpid() || bootstrapProcessAlive(pid) {
		return "", nil
	}
	fmt.Fprintf(os.Stderr, "bootstrap trust-plane teardown: adopting orphaned marker from dead pid %d\n", pid)
	if adoptionErr := destroyOrphanedBootstrapMarkers(marker); adoptionErr != nil {
		// The orphan could not be destroyed: keep the marker so the next
		// package run fails closed, and surface the failure visibly. The
		// distinct non-nil error is propagated to the caller so the
		// production TestMain path fails closed instead of proceeding.
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane teardown: orphan adoption FAILED (marker kept): %v\n", adoptionErr)
		return "", fmt.Errorf("orphan adoption failed, marker kept: %w", adoptionErr)
	}
	if runtimeBootstrapAdoptionHook != nil {
		runtimeBootstrapAdoptionHook()
	}
	if err := os.Remove(marker); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption marker removal: %v\n", err)
	}
	return marker, nil
}

// bootstrapProcessAlive reports whether the given pid currently exists.
func bootstrapProcessAlive(pid int) bool {
	if pid <= 1 || pid == os.Getpid() {
		return false
	}
	// Signal 0 probes existence without delivering anything. Note: as an
	// unprivileged user, EPERM also reports an existing process (the Go
	// helper's err==nil check would be wrong for foreign-uid pids).
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// destroyOrphanedBootstrapMarkers destroys, without claiming package
// ownership, every configured bootstrap target this process did not provision
// but whose marker proves it belongs to this repository's generated
// namespace. Inventory and destroy failures are reported fail-closed and
// aggregated into the returned error: a config-load error, an unread marker,
// a missing persisted target, an unresolvable provider or any destroy failure
// all make the return value non-nil, so the caller keeps the marker and fails
// closed instead of consuming a claim it could not actually destroy.
func destroyOrphanedBootstrapMarkers(marker string) error {
	var errs []error
	// Injected-failure regression hook: when set, the orphan destroy fails
	// before destroying anything so the fail-closed adoption wiring is
	// exercised for real — the marker must be kept and the run fails closed.
	if runtimeBootstrapOrphanDestroyFailure != nil {
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption orphan destroy failed: %v\n", runtimeBootstrapOrphanDestroyFailure)
		return fmt.Errorf("injected orphan-destroy failure: %w", runtimeBootstrapOrphanDestroyFailure)
	}
	// Adoption is state-driven: the persisted configuration tells which
	// bootstrap targets were created; the provider is re-derived from it. A
	// process that died before `target create` persisted nothing to destroy,
	// which is reported explicitly instead of pretending success.
	cfg, err := deployment.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption cannot load config: %v\n", err)
		return err
	}
	found := false
	defer func() {
		if !found {
			fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption found no persisted bootstrap target for marker %s\n", marker)
		}
	}()
	owner, ownerErr := os.ReadFile(marker)
	if ownerErr != nil {
		fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption cannot read marker: %v\n", ownerErr)
		return fmt.Errorf("bootstrap trust-plane adoption cannot read marker: %w", ownerErr)
	}
	for name, target := range cfg.Targets {
		if !bootstrapTargetPattern.MatchString(name) || target.Runtime.Provider == "" {
			continue
		}
		markerOwner := strings.TrimSpace(string(owner))
		found = true
		claimed := runtimeIntegrationTrustPlaneTargetName
		claimedTarget := runtimeIntegrationTrustPlaneTarget
		claimedProvider := runtimeIntegrationTrustPlaneProvider
		runtimeIntegrationTrustPlaneTargetName = name
		provider, resolved, perr := bootstrapTeardownProvider()
		if perr != nil {
			fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption target %q: %v\n", name, perr)
			errs = append(errs, fmt.Errorf("bootstrap trust-plane adoption target %q: %w", name, perr))
			runtimeIntegrationTrustPlaneTargetName = claimed
			continue
		}
		runtimeIntegrationTrustPlaneTarget = resolved
		runtimeIntegrationTrustPlaneProvider = provider
		// A matched target only makes the adoption fatal when it actually
		// owns a provisioned Core. A dead owner that wrote the marker and a
		// config entry but died before `up` ever brought up the Core leaves
		// neither compose files nor owned containers behind. That is not a
		// residual to reclaim and not a live owner: it is a phantom target
		// whose claim is safe to consume. Fail-closed is reserved for a
		// genuine residual (owned containers/files still exist after
		// destroy) or a provider error while something WAS provisioned —
		// never for a target that was never brought up.
		inventoryCtx, inventoryCancel := backgroundInventoryCtx()
		orphansProvisioned, provErr := orphanBootstrapTargetHadFootprint(
			inventoryCtx, provider, resolved)
		inventoryCancel()
		if provErr != nil {
			fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption target %q footprint: %v\n", name, provErr)
			errs = append(errs, fmt.Errorf("bootstrap trust-plane adoption target %q footprint: %w", name, provErr))
		} else if !orphansProvisioned {
			fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption consumed orphaned marker %q: target %q owned no provisioned Core (dead owner never brought it up)\n", marker, name)
			runtimeIntegrationTrustPlaneTargetName = claimed
			runtimeIntegrationTrustPlaneTarget = claimedTarget
			runtimeIntegrationTrustPlaneProvider = claimedProvider
			continue
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		derr := destroyRuntimeIntegrationTrustPlane(cleanupCtx)
		cancel()
		if derr != nil {
			fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption destroyed target %q with reported failures: %v\n", name, derr)
			errs = append(errs, fmt.Errorf("bootstrap trust-plane adoption destroyed target %q with failures: %w", name, derr))
		} else {
			fmt.Fprintf(os.Stderr, "bootstrap trust-plane adoption destroyed orphaned target %q (dead marker owner %s)\n", name, markerOwner)
		}
		runtimeIntegrationTrustPlaneTargetName = claimed
		runtimeIntegrationTrustPlaneTarget = claimedTarget
		runtimeIntegrationTrustPlaneProvider = claimedProvider
	}
	return errors.Join(errs...)
}

// backgroundInventoryCtx returns a bounded background context and its cancel
// function for the dead-owner footprint inventory. Ambient test cancellation
// must never abort a teardown-class check, so it never derives from the test
// context. The cancel function must be called by the caller AFTER the caller's
// inventory has finished: returning a context that is already canceled (e.g. by
// deferring cancel in this helper) would abort the inventory it exists to run,
// which is exactly the phantom-footprint defect this fix closes.
func backgroundInventoryCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	return ctx, cancel
}

// orphanBootstrapTargetHadFootprint reports whether a dead owner's matched
// bootstrap target actually owned a provisioned Core — i.e. whether the run's
// namespace for that target had runtime project files (compose.yaml + env)
// or owned containers/resources on the engine. A target whose config entry
// was persisted but whose Core was never brought up (the torn teardown/sequence
// this adoption fix closes) leaves neither; that orphan is a phantom claim
// with nothing to reclaim, so adoption consumes it cleanly instead of failing
// the whole package run. Fail-closed is reserved for a real residual: when
// something WAS provisioned and owned resources remain after destroy, or the
// engine inventory itself errors while a Core existed. A live owner's Core is
// never touched — the dead-owner gate (bootstrapProcessAlive) is what reaches
// this path, so footprint alone never re-opens live-owner protection.
func orphanBootstrapTargetHadFootprint(ctx context.Context, provider bhruntime.RuntimeProvider, target deployment.ResolvedTarget) (bool, error) {
	if provider == nil {
		// A provider we cannot derive is unverifiable: fail closed rather
		// than silently consuming a claim we could not actually inspect.
		return false, fmt.Errorf("orphan footprint provider is nil for target %q", target.Name)
	}
	dataDir, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		// A target root we cannot resolve is unverifiable: fail closed
		// rather than silently consume a claim we could not inspect.
		return false, fmt.Errorf("resolve target root for orphan footprint: %w", err)
	}
	runtimeRoot := filepath.Join(dataDir, "runtime")
	// Runtime project files are the authoritative proof a Core was provisioned:
	// compose.yaml + env exist only after a successful `up`.
	//
	// Test seam: when runtimeBootstrapOrphanFileReadErr is set, the file read
	// returns that synthetic error so the permission/I/O branch of the
	// fail-closed adoption is exercised for real — a mutation that treated an
	// unreadable runtime directory as "never provisioned" would consume the
	// marker here instead of keeping it.
	_, filesErr := bootstrapOrphanReadRuntimeFiles(runtimeRoot, target.Name)
	if filesErr != nil {
		// Only a genuine not-found (files the engine never wrote) proves no
		// provisioned Core. A permission or I/O error here is unverifiable:
		// fail closed and keep the marker instead of consuming a claim we
		// could not actually inspect.
		if !errors.Is(filesErr, os.ErrNotExist) {
			return false, fmt.Errorf("read runtime files for orphan target %q: %w", target.Name, filesErr)
		}
		// No project files means no provisioned Core. Confirm there is also
		// nothing owned on the engine before treating it as a phantom.
		owned, invErr := ownedFootprintOnEngine(ctx, provider, target, dataDir)
		if invErr != nil {
			return false, fmt.Errorf("inventory engine footprint for orphan target %q: %w", target.Name, invErr)
		}
		return owned, nil
	}
	return true, nil
}

// bootstrapOrphanReadRuntimeFiles returns the runtime project files for the
// dead-owner footprint inventory. It is a thin indirection over
// ExistingFilesForProject so the permission/I/O regression can inject a
// synthetic read error deterministically without touching the real filesystem.
func bootstrapOrphanReadRuntimeFiles(runtimeRoot, projectName string) (bhruntime.Files, error) {
	if runtimeBootstrapOrphanFileReadErr != nil {
		return bhruntime.Files{}, runtimeBootstrapOrphanFileReadErr
	}
	return bhruntime.ExistingFilesForProject(runtimeRoot, bhruntime.SharedProjectName(projectName))
}

// containers or project resources for the target's namespace. Only used to
// confirm a target with no compose files truly owns nothing; a real residual
// (containers/resources present) still fails closed downstream.
func ownedFootprintOnEngine(ctx context.Context, provider bhruntime.RuntimeProvider, target deployment.ResolvedTarget, dataDir string) (bool, error) {
	if owned := bootstrapOwnedProjects(target.Name, dataDir); len(owned) != 0 {
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			// Engine inventory is unavailable: fail closed rather than
			// mistaking a dead owner's real Core for a phantom claim.
			return false, fmt.Errorf("list runtime containers for orphan target %q: %w", target.Name, err)
		}
		for _, c := range containers {
			if bootstrapContainerOwned(c.Project, c.Name, owned, target.Name) {
				return true, nil
			}
		}
	}
	projects := make([]string, 0, len(bootstrapOwnedProjects(target.Name, dataDir)))
	for project := range bootstrapOwnedProjects(target.Name, dataDir) {
		projects = append(projects, project)
	}
	for _, project := range projects {
		left, err := provider.ListOwnedProjectResources(ctx, project)
		if err != nil {
			// Engine inventory is unavailable: fail closed rather than
			// mistaking a dead owner's real Core for a phantom claim.
			return false, fmt.Errorf("list owned project resources for orphan target %q project %q: %w", target.Name, project, err)
		}
		if len(left) != 0 {
			return true, nil
		}
	}
	return false, nil
}

// completion (success or reported residuals). The marker survives a teardown
// that could not even begin, so the next package run fails closed instead of
// silently keeping a residual Core. It uses its OWN fresh context: ambient
// test cancellation must never prevent teardown.
func destroyBootstrapTrustPlane(marker string) error {
	if marker == "" {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var errs []error
	// Run the teardown whenever this process claimed a bootstrap target OR a
	// provider was captured; a claimed target with an unresolved provider
	// still tears down through the persisted configuration (fail-closed).
	if bootstrapTeardownClaimed() {
		if destroyErr := destroyRuntimeIntegrationTrustPlane(cleanupCtx); destroyErr != nil {
			// Distinguish "provisioning never completed" (config/runtime
			// unavailable, no daemon-side proof required) from a genuine
			// residual or destroy failure: the former is only notable once
			// the wächter itself could not run at all.
			errs = append(errs, destroyErr)
			// The marker is only consumed when the destroy ran to completion
			// AND the residual watcher found nothing left to reclaim. A destroy
			// that ran but reported residuals still consumed the marker under
			// the r3 contract (stage one: "ran and reported residuals" ->
			// marker gone); the marker-survives path is the failed-ADOPTION
			// path tested in stage two, NOT this package-teardown run.
			if isUnbeginningTeardownError(destroyErr) {
				return fmt.Errorf("bootstrap trust-plane teardown could not begin: %w", destroyErr)
			}
		}
	}
	errs = append(errs, os.Remove(marker))
	return errors.Join(errs...)
}

// isUnbeginningTeardownError reports whether the teardown error means the
// wächter could not even run — provisioning never completed, the config is
// unavailable, the target is unconfigured or the runtime provider could not be
// resolved. In those cases the marker must survive so the next run fails
// closed instead of forgetting the orphan. A destroy that ran but reported
// residuals is NOT unbeginning and the marker is consumed.
func isUnbeginningTeardownError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "teardown config") ||
		strings.Contains(msg, "teardown has no target identity") ||
		strings.Contains(msg, "teardown target") ||
		strings.Contains(msg, "teardown runtime provider") ||
		strings.Contains(msg, "could not load config")
}

func ensureRuntimeIntegrationTrustPlane(t *testing.T, ctx context.Context) {
	t.Helper()
	runtimeIntegrationTrustPlaneOnce.Do(func() {
		// Capture the data root BEFORE any XDG_DATA_HOME switch below. TestMain
		// claims the durable provisioning marker under the initial data root; if
		// this helper unconditionally pointed XDG_DATA_HOME at a fresh temp
		// dir, the marker the teardown later re-reads would live under a
		// different root than the one that claimed it and the run would fail
		// with "bootstrap leftover marker missing". We only switch the data
		// root when the claim is carried along with us (its path is recorded
		// below), so the teardown and the trust plane share one root.
		markerPathBeforeSwitch := bootstrapLeftoverMarkerPath()
		configHome, err := os.MkdirTemp("", "baseharbor-ci-config-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		dataHome, err := os.MkdirTemp("", "baseharbor-ci-data-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// If a durable claim already exists under the initial root, move it
		// into the new per-run data home so both the trust-plane setup and the
		// package teardown observe the same root. A missing claim (a run that
		// never provisioned) needs no move.
		if markerPathBeforeSwitch != "" {
			if _, statErr := os.Stat(markerPathBeforeSwitch); statErr == nil {
				newMarker, mErr := moveMarkerToRoot(markerPathBeforeSwitch, dataHome)
				if mErr != nil {
					runtimeIntegrationTrustPlaneErr = fmt.Errorf("carry bootstrap marker into per-run data home: %w", mErr)
					return
				}
				if newMarker != "" {
					markerPathBeforeSwitch = newMarker
				}
			}
		}
		if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("XDG_DATA_HOME", dataHome); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Record the data root the trust plane now owns so ensureBootstrapLeftoverMarker
		// re-reads the marker under exactly this root.
		runtimeIntegrationTrustPlaneDataRoot = dataHome
		// Guarantee the durable marker exists under the NEW data home claimed by
		// this process. On a runner where neither BASEHARBOR_STATE_DIR nor
		// XDG_DATA_HOME is set, TestMain's marker lives under the DataRoot
		// (HOME) fallback; after switching XDG_DATA_HOME to dataHome the teardown
		// would otherwise find nothing and fail with "bootstrap leftover marker
		// missing". Writing THIS process's PID here makes the marker-owned-by-us
		// assertion pass regardless of where TestMain created its original claim.
		if _, statErr := os.Stat(filepath.Join(dataHome, bootstrapLeftoverMarkerName)); statErr != nil {
			claimBootstrapLeftoverMarker(filepath.Join(dataHome, bootstrapLeftoverMarkerName))
		}
		// Isolation is a CODE-GARANTIE, not an environmental precondition: the
		// bootstrap target below is a *fresh* configured target with its own
		// `<name>-default` native network and its own management Core, so this
		// single shared `up --control-plane-only` never gates on the ambient
		// `local` Core that a polluted shared daemon may carry. The ambient
		// `local` Core (if any) is never touched, reset or deleted. The four
		// lifecycle tests below each add their own distinct BASEHARBOR_TARGET
		// after this helper runs (one isolated Core per test), so a mutation
		// that drops a single test's target fails only that one test at the
		// gate — a per-test discriminating proof, not a systemic failure.
		//
		// The bootstrap target name is generated fresh per process so a
		// contaminated or pre-existing Core on a fixed name can never be the
		// isolation boundary: each run owns its own `<name>-default` network and
		// management Core. A per-process suffix keeps the cell reproducible only
		// by re-running, never by reusing another lane's fixed-name Core.
		bootstrapTarget := "w169-ci-bootstrap-" + bootstrapTargetSuffix()
		// Record ownership of this generated namespace BEFORE any operation
		// that can materialize resources. The package teardown destroys what
		// this process owns even when the failure happens before the provider
		// could be resolved; TestMain claimed the durable marker before the
		// package run started, and ensureBootstrapLeftoverMarker verifies that
		// claim instead of silently skipping teardown (fail-safe: provisioning
		// is noted only while teardown is guaranteed).
		runtimeIntegrationTrustPlaneTargetName = bootstrapTarget
		if err := ensureBootstrapLeftoverMarker(); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if err := os.Setenv("BASEHARBOR_TARGET", bootstrapTarget); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Always restore the selection, including when OpenBao was already
		// initialized or when bootstrap fails. Otherwise a later lifecycle
		// test silently inherits this bootstrap target.
		defer func() {
			if err := os.Unsetenv("BASEHARBOR_TARGET"); err != nil && runtimeIntegrationTrustPlaneErr == nil {
				runtimeIntegrationTrustPlaneErr = err
			}
			if runtimeIntegrationTrustPlaneErr == nil {
				runtimeIntegrationTrustPlaneErr = mutatePersistedTarget("", true)
			}
		}()
		// The shared rootless daemon is the integration endpoint. An active
		// CLI context (the prepare step's setuptool registers "rootless")
		// takes precedence over DOCKER_HOST inside the engine resolver, so
		// make the endpoint authoritative and neutralize the context.
		_ = os.Unsetenv("DOCKER_CONTEXT")

		var out bytes.Buffer
		if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
			if err := runWithIO(ctx, []string{"target", "create", bootstrapTarget, "--runtime-provider", "podman", "--access", "local", "--access-provider", "local", "--reference", "local", "--default"}, &out, &out); err != nil {
				runtimeIntegrationTrustPlaneErr = err
				return
			}
			out.Reset()
		} else {
			// Docker (the default test runtime) needs its own bootstrap target
			// configured before the single shared `up` below; this target has
			// its own `<name>-default` native network and management Core, so
			// the gate never sees the ambient `local` Core.
			if err := runWithIO(ctx, []string{"target", "create", bootstrapTarget, "--runtime-provider", "docker", "--access", "local", "--access-provider", "local", "--reference", "local", "--default"}, &out, &out); err != nil {
				runtimeIntegrationTrustPlaneErr = err
				return
			}
			out.Reset()
		}
		if err := runWithIO(ctx, []string{"up", "--control-plane-only", "--yes"}, &out, &out); err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		// Capture the bootstrap Core runtime provider + owning target NOW, while
		// the bootstrap target selection and its config are still valid. The
		// deferred unset below clears BASEHARBOR_TARGET, so teardown cannot
		// re-resolve them later. This stored pair is the fast path of the
		// single package teardown; if resolution fails, teardown still owns
		// the target through the marker name and re-derives the provider from
		// the persisted target configuration. A capture error is never
		// swallowed: it is carried and the teardown path fails closed.
		tgt, terr := effectiveTarget(ctx)
		if terr != nil {
			runtimeIntegrationTrustPlaneErr = fmt.Errorf("capture bootstrap trust-plane owning target: %w", terr)
			return
		}
		provider, perr := detectRuntimeForTarget(ctx, tgt)
		if perr != nil {
			runtimeIntegrationTrustPlaneErr = fmt.Errorf("capture bootstrap trust-plane runtime provider: %w", perr)
			return
		}
		runtimeIntegrationTrustPlaneTarget = tgt
		runtimeIntegrationTrustPlaneProvider = provider

		compose, files, err := openBaoRuntime(ctx)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		state, err := platformopenbao.Inspect(ctx, compose, files)
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		if state.Initialized {
			if state.Sealed {
				runtimeIntegrationTrustPlaneErr = os.ErrPermission
				return
			}
			runtimeIntegrationTrustPlaneErr = platformopenbao.CheckManager(ctx, compose, files)
			return
		}

		dir, err := os.MkdirTemp("", "baseharbor-ci-openbao-")
		if err != nil {
			runtimeIntegrationTrustPlaneErr = err
			return
		}
		recoveryPath := filepath.Join(dir, "recovery.json")
		out.Reset()
		runtimeIntegrationTrustPlaneErr = runWithIO(ctx, []string{"openbao", "bootstrap", "--recovery-file", recoveryPath}, &out, &out)
	})
	if runtimeIntegrationTrustPlaneErr != nil {
		t.Fatalf("prepare BaseHarbor integration trust plane: %v", runtimeIntegrationTrustPlaneErr)
	}
	if _, err := existingTargetRuntimeFiles(ctx); err != nil {
		t.Fatalf("BaseHarbor integration trust plane state missing: %v", err)
	}
	// A missing per-test isolation call must never silently fall back to
	// the bootstrap Core or a configured default target.
	sentinel := "w169-isolation-required-" + bootstrapTargetSuffix()
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load targets for isolation guard: %v", err)
	}
	if _, exists := cfg.Targets[sentinel]; exists {
		t.Fatalf("isolation guard target %q unexpectedly exists", sentinel)
	}
	t.Setenv("BASEHARBOR_TARGET", sentinel)

	// No t.Cleanup here on purpose: the teardown of the shared singleton is
	// owned by TestMain and runs exactly once after the whole package, not at
	// the end of the first helper caller. Setup failures (t.Fatalf above) are
	// covered because the provisioning marker already guarantees teardown.
}

// ensureBootstrapLeftoverMarker makes teardown guarantee explicit: the marker
// must exist (created by TestMain before any provisioning can start) and must
// have been written by this process. A missing or foreign marker means
// provisioning could finish without a guaranteed teardown, so the helper fails
// closed instead of proceeding.
func ensureBootstrapLeftoverMarker() error {
	marker := bootstrapLeftoverMarkerPath()
	if marker == "" {
		return fmt.Errorf("bootstrap leftover marker unavailable: no BaseHarbor data root is configured")
	}
	owner, err := os.ReadFile(marker)
	if err != nil {
		return fmt.Errorf("bootstrap leftover marker missing (teardown not guaranteed): %w", err)
	}
	if strings.TrimSpace(string(owner)) != strconv.Itoa(os.Getpid()) {
		return fmt.Errorf("bootstrap leftover marker was not written by this process: %q", strings.TrimSpace(string(owner)))
	}
	return nil
}

// destroyRuntimeIntegrationTrustPlane tears down exactly the bootstrap Core
// this process provisioned: the consolidated control-plane project plus its
// shared keycloak Core and shared application backends. It is fail-closed —
// every residual in this run's namespace, every teardown error and every
// inventory error is returned as an aggregated failure, which the caller
// surfaces as a test error (t.Errorf) or a package-level failure (stderr).
// The wächter inventory (assertBootstrapTrustPlaneLeavesNoLeftover) always
// runs, even when a destroy step failed, so residuals are never silently
// dropped. No product destroy path is altered; only resources the test suite
// materialized are removed. The owning target + runtime are the pair captured
// during setup; when setup failed before the provider could be resolved,
// bootstrapTeardownProvider re-derives both from the persisted target the
// marker names, and a provider that cannot be resolved at all still runs the
// residual inventory.
func destroyRuntimeIntegrationTrustPlane(ctx context.Context) error {
	// Injected-failure regression hook: when set, the teardown fails before
	// destroying anything so the FAIL-OPEN wiring is exercised for real — the
	// package teardown must surface the error as a non-zero exit and the
	// orphan path must keep the marker instead of consuming the claim.
	if runtimeBootstrapTeardownFailure != nil {
		return fmt.Errorf("injected bootstrap trust-plane teardown failure: %w", runtimeBootstrapTeardownFailure)
	}
	var errs []error
	provider := runtimeIntegrationTrustPlaneProvider
	target := runtimeIntegrationTrustPlaneTarget
	if provider == nil {
		var err error
		provider, target, err = bootstrapTeardownProvider()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if target.Name == "" {
		target.Name = runtimeIntegrationTrustPlaneTargetName
	}
	if provider != nil {
		dataDir, err := deployment.TargetStateRoot(target.Name)
		if err != nil {
			errs = append(errs, fmt.Errorf("bootstrap trust-plane teardown target root: %w", err))
			return errors.Join(errs...)
		}
		if runtimeBootstrapTeardownObserver != nil {
			runtimeBootstrapTeardownObserver(target.Name, dataDir)
		}
		runCleanup := func(resource string, fn func() error) {
			if err := fn(); err != nil {
				errs = append(errs, fmt.Errorf("bootstrap trust-plane teardown %s: %w", resource, err))
			}
		}
		// Every destroy step runs even when an earlier one failed, so a
		// partial failure can never skip the resources of the later phases.
		runCleanup("identity-keycloak", func() error {
			return identityprovider.DestroyAllSharedKeycloakAt(ctx, provider, dataDir, target.Name)
		})
		runCleanup("data-providers", func() error {
			return application.DestroyAllSharedBackendsAt(ctx, provider, dataDir, target.Name)
		})
		runtimeRoot := filepath.Join(dataDir, "runtime")
		files, filesErr := bhruntime.ExistingFilesForProject(runtimeRoot, bhruntime.SharedProjectName(target.Name))
		if filesErr != nil {
			// Inventory errors are fail-closed: they never silently skip the
			// control-plane destroy step of the run's own namespace.
			errs = append(errs, fmt.Errorf("bootstrap trust-plane control-plane files: %w", filesErr))
		} else {
			runCleanup("control-plane", func() error {
				return provider.DestroyProject(ctx, files.Project, files.Compose, files.Env)
			})
		}
	}
	if invErr := assertBootstrapTrustPlaneLeavesNoLeftover(ctx, provider, target); invErr != nil {
		errs = append(errs, fmt.Errorf("bootstrap trust-plane teardown left residual resources: %w", invErr))
	}
	return errors.Join(errs...)
}

// bootstrapTeardownProvider re-derives the owning target and runtime provider
// from the persisted configuration when setup failed before capture. The
// bootstrap target is created --default, so its name survives in the config
// even when the process died between `up` and the capture step.
func bootstrapTeardownProvider() (bhruntime.RuntimeProvider, deployment.ResolvedTarget, error) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return nil, deployment.ResolvedTarget{}, fmt.Errorf("bootstrap trust-plane teardown config: %w", err)
	}
	name := runtimeIntegrationTrustPlaneTargetName
	if name == "" {
		return nil, deployment.ResolvedTarget{}, fmt.Errorf("bootstrap trust-plane teardown has no target identity")
	}
	if _, defined := cfg.Targets[name]; !defined {
		// A missing definition means the target was never persisted, so
		// ownership is unverifiable and the teardown fails closed.
		return nil, deployment.ResolvedTarget{}, fmt.Errorf("bootstrap trust-plane teardown target %q is not configured", name)
	}
	resolved, err := cfg.ResolveTarget(name, "")
	if err != nil {
		return nil, deployment.ResolvedTarget{}, fmt.Errorf("bootstrap trust-plane teardown target resolution: %w", err)
	}
	if runtimeBootstrapOrphanInventoryProvider != nil {
		// Injected-failure regression: supply a provider the inventory can
		// fail through so fail-closed adoption is exercised for real.
		return runtimeBootstrapOrphanInventoryProvider, resolved, nil
	}
	provider, err := detectRuntimeForTarget(context.Background(), resolved)
	if err != nil {
		return nil, deployment.ResolvedTarget{}, fmt.Errorf("bootstrap trust-plane teardown runtime provider: %w", err)
	}
	return provider, resolved, nil
}

// bootstrapDataDirForTarget resolves the state root the teardown and the
// wächter share; an unresolvable root only weakens the (additive)
// state-discovery scope, the static namespaces still apply.
// bootstrapDataDirForTarget resolves the state root the teardown and the
// wächter share: the persisted TargetStateRoot for the run's own bootstrap
// target. When provisioning failed before any state was persisted — or the
// package-coverage fixtures run without the deployment data root that the
// real cell provides — the process-local package root from
// setupBootstrapLeftoverMarker scopes discovery instead.
func bootstrapDataDirForTarget(name string) string {
	if name != "" {
		if root, err := deployment.TargetStateRoot(name); err == nil && bootstrapTargetStateMaterialized(root) {
			return root
		}
	}
	return bootstrapPackageRoot()
}

// bootstrapTargetStateMaterialized reports whether the target's state
// directory exists.
func bootstrapTargetStateMaterialized(root string) bool {
	_, err := os.Stat(root)
	return err == nil
}

// bootstrapPackageRoot is the data root this process uses for the durable
// provisioning claim; it doubles as the fixture fallback.
func bootstrapPackageRoot() string {
	if bootstrapPackageRootOverride != "" {
		return bootstrapPackageRootOverride
	}
	root, err := deployment.DataRoot()
	if err != nil {
		return ""
	}
	return root
}

// bootstrapSetPackageRootForTest pins the package fallback root for the
// package-coverage fixtures (the process-global env of a real cell run is
// set by setup; fixtures model it without racing other tests' env restores).
var bootstrapPackageRootOverride string

func bootstrapSetPackageRootForTest(root string) func() {
	previous := bootstrapPackageRootOverride
	bootstrapPackageRootOverride = root
	return func() { bootstrapPackageRootOverride = previous }
}

// assertBootstrapTrustPlaneLeavesNoLeftover is the package wächter: it fails
// when any BaseHarbor-owned resource in THIS run's generated namespace
// (containers, volumes or networks of the bootstrap control-plane project and
// its explicitly derived shared subprojects) survives teardown. Inventory
// errors are returned, never swallowed, so the run fails closed when the
// residual class cannot be inspected.
func assertBootstrapTrustPlaneLeavesNoLeftover(ctx context.Context, provider bhruntime.RuntimeProvider, target deployment.ResolvedTarget) error {
	if target.Name == "" {
		target.Name = runtimeIntegrationTrustPlaneTargetName
	}
	if target.Name == "" {
		return fmt.Errorf("wächter cannot scope residuals: this run owns no bootstrap target name")
	}
	if provider == nil {
		return fmt.Errorf("wächter cannot inspect residuals: no runtime provider available for target %q", target.Name)
	}
	owned := bootstrapOwnedProjects(target.Name, bootstrapDataDirForTarget(target.Name))
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return fmt.Errorf("inspect containers after bootstrap teardown: %w", err)
	}
	residual := []string{}
	for _, c := range containers {
		if bootstrapContainerOwned(c.Project, c.Name, owned, target.Name) {
			residual = append(residual, c.Project+"/"+c.Name)
		}
	}
	if len(residual) != 0 {
		sort.Strings(residual)
		return fmt.Errorf("bootstrap containers remain after teardown: %s", strings.Join(residual, ", "))
	}
	names := make([]string, 0, len(owned))
	for project := range owned {
		names = append(names, project)
	}
	sort.Strings(names)
	if runtimeBootstrapWatcherObserver != nil {
		runtimeBootstrapWatcherObserver(names)
	}
	for _, project := range names {
		leftover, err := provider.ListOwnedProjectResources(ctx, project)
		if err != nil {
			return fmt.Errorf("inspect bootstrap resources for project %s after teardown: %w", project, err)
		}
		if len(leftover) == 0 {
			continue
		}
		residual = residual[:0]
		for _, r := range leftover {
			residual = append(residual, r.Kind+" "+r.Name)
		}
		sort.Strings(residual)
		return fmt.Errorf("bootstrap resources remain after teardown in project %s: %s", project, strings.Join(residual, ", "))
	}
	return nil
}

// bootstrapOwnedProjects enumerates exactly the Compose project namespaces this
// run generated: the consolidated control-plane project of its bootstrap
// target plus the explicitly derived shared data/identity subprojects of the
// same target. Names are derived from the runtime's own project-name contract,
// never from a broad lane prefix, so a concurrently running lane's randomly
// suffixed projects can never be counted (or destroyed) here.
// bootstrapOwnedProjects enumerates exactly the Compose project namespaces
// this run generated: the consolidated control-plane project of its bootstrap
// target plus the shared provider resources that setup materialized — shared
// data backends and shared Keycloak. Membership is discovered from the SAME
// on-disk state directories the destroy steps walk (providers/… and
// runtime/compose.yaml), so watcher scope equals destroy scope; with no
// materialized state the minimal derived namespace still applies. Names come
// from the runtime's own project-name contract, never from a broad lane
// prefix, so a concurrently running lane's randomly suffixed projects can
// never be counted (or destroyed) here.
// bootstrapContainerOwned reports whether the container belongs to this run's
// namespaces, identified by the compose project label (the single source of
// truth shared with ListOwnedProjectResources). A container WITHOUT any
// compose project label can only be attributed to this run when its name
// carries this run's target token (Compose-derived names start with
// `<project>-`, physical resource names embed `baseharbor-<token>-`); a
// foreign token never matches, so other lanes' unlabeled containers are
// never counted.
func bootstrapContainerOwned(project, name string, owned map[string]bool, targetName string) bool {
	if owned[project] {
		return true
	}
	if project != "" || targetName == "" {
		return false
	}
	token := targetTokenOf(targetName)
	return strings.HasPrefix(name, "bh-"+token) || strings.HasPrefix(name, "baseharbor-"+token)
}

// targetTokenOf returns the normalized project token the runtime derives for
// a target namespace (mirrors runtime.projectToken).
func targetTokenOf(namespace string) string {
	token := strings.ToLower(strings.TrimSpace(namespace))
	token = strings.ReplaceAll(token, ".", "-")
	token = strings.Trim(token, "-")
	if token == "" {
		token = "local"
	}
	return token
}

func bootstrapOwnedProjects(targetName string, dataDir string) map[string]bool {
	// The control-plane destroy is driven by the persisted runtime files, so
	// the operator-visible shared project is only in scope once this run's
	// target actually materialized them.
	projects := map[string]bool{}
	if dataDir == "" {
		return projects
	}
	derived := map[string]bool{}
	// The control-plane namespace is in scope whether or not the state
	// directory was materialized: DestroyProject is the fail-closed residual
	// remover and the watcher must cover the namespace the run generated.
	derived[bhruntime.SharedProjectName(targetName)] = true
	if _, err := os.Stat(filepath.Join(dataDir, "runtime", "compose.yaml")); err == nil {
		derived[bhruntime.SharedProjectName(targetName)] = true
	}
	// The consolidated shared data-backend namespace belongs to the target
	// once its provider state directory exists.
	if entries, err := os.ReadDir(filepath.Join(dataDir, "providers", "shared-backends")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				derived[application.SharedBackendFilesAt(dataDir, targetName, entry.Name()).Project] = true
			}
		}
	}
	// Keycloak boundaries are discovered state, not assumed: the core
	// boundary installs under the `<target>-core` shared project.
	if _, err := os.Stat(filepath.Join(dataDir, "providers", "keycloak", "shared", "core", "compose.yaml")); err == nil {
		derived[bhruntime.SharedProjectName(targetName+"-core")] = true
	}
	if _, err := os.Stat(filepath.Join(dataDir, "providers", "keycloak", "shared", "replica", "compose.yaml")); err == nil {
		derived[bhruntime.SharedProjectName(targetName)] = true
	}
	// Shared data backends are consolidated under the operator-visible
	// shared project of the target itself.
	if _, err := os.Stat(filepath.Join(dataDir, "providers", "shared-backends")); err == nil {
		derived[bhruntime.SharedProjectName(targetName)] = true
	}
	for project := range derived {
		// Ownership must be attributable to THIS run: a derived namespace
		// whose target token is not this run's generated bootstrap token
		// would overlap foreign state and is never claimed (fail-closed).
		if projectTargetName(project) == targetName || strings.TrimSuffix(projectTargetName(project), "-core") == targetName {
			projects[project] = true
		}
	}
	if len(projects) == 0 && dataDir != "" {
		// Nothing materialized: the watcher keeps the minimal derived
		// namespace set so an inventory-only pass is still possible.
		return map[string]bool{bhruntime.SharedProjectName(targetName): true}
	}
	return projects
}

// projectTargetName recovers the namespace a `bh-<namespace>-shared`
// consolidated project was derived from. The namespace token may itself
// contain hyphens, so the trailing `-shared` marker is the only reliable
// boundary.
func projectTargetName(project string) string {
	token := strings.TrimSuffix(strings.TrimPrefix(project, "bh-"), "-shared")
	return token
}

// runtimeIntegrationTrustPlaneCleanupForTest is the durable package-end
// teardown entry point used by the regression tests: it mirrors exactly what
// TestMain does after m.Run(), against a marker file the caller supplies.
func runtimeIntegrationTrustPlaneCleanupForTest(t *testing.T, marker string) {
	t.Helper()
	if err := destroyBootstrapTrustPlane(marker); err != nil {
		t.Errorf("bootstrap trust-plane teardown left residual resources: %v", err)
	}
}

// isolateRuntimeForTest configures a fresh target for the requested runtime.
// Target and provider are verified through the real deployment resolver, not
// inferred from environment variables or constructed network names.
func isolateRuntimeForTest(t *testing.T, name string) {
	t.Helper()
	provider := integrationRuntimeProvider(t)
	isoTarget := "w169-" + name + "-" + bootstrapTargetSuffix() + "-iso"
	if err := os.Setenv("BASEHARBOR_TARGET", isoTarget); err != nil {
		t.Fatalf("set isolated target %q: %v", isoTarget, err)
	}
	var buf bytes.Buffer
	args := []string{"target", "create", isoTarget, "--runtime-provider", provider, "--access", "local", "--access-provider", "local", "--reference", "local"}
	if err := runWithIO(t.Context(), args, &buf, &buf); err != nil {
		t.Fatalf("configure isolated %s target %q: %v\n%s", provider, isoTarget, err, buf.String())
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatalf("load actual target configuration: %v", err)
	}
	selected, err := selectedTargetName("", os.Getenv("BASEHARBOR_TARGET"), cfg)
	if err != nil {
		t.Fatalf("resolve active target: %v", err)
	}
	resolved, err := cfg.ResolveTarget(selected, "")
	if err != nil {
		t.Fatalf("resolve configured runtime target: %v", err)
	}
	if resolved.Name != isoTarget || resolved.RuntimeProvider != provider || resolved.AccessProvider != "local" {
		t.Fatalf("runtime isolation mismatch: want target=%q provider=%q access=local, got %+v", isoTarget, provider, resolved)
	}
	// TEST-SCOPE owner loan marker (card t-20261106-r27-remote-up-podman-owner-missing,
	// lane w286): this isolated test Target has no installation of its own by
	// design — the boot Target's genuine Core installation is the single
	// durable owner. The marker file documents the isolated test Target; the
	// production owner loan resolves this namespace against the boot Core by
	// its genuine, READY installation, not by the marker alone.
	writeBahaTestIsolationMarker(t, isoTarget)
	t.Logf("isolated runtime resolved from deployment config: target=%q runtime=%q endpoint=%q context=%q", resolved.Name, resolved.RuntimeProvider, resolved.DockerEndpoint, resolved.DockerContext)
}

func integrationRuntimeProvider(t *testing.T) string {
	t.Helper()
	provider := strings.TrimSpace(os.Getenv("BASEHARBOR_TEST_RUNTIME"))
	if provider == "" {
		return "docker"
	}
	if provider != "docker" && provider != "podman" {
		t.Fatalf("unsupported integration runtime provider %q", provider)
	}
	return provider
}

// bootstrapTargetPattern matches exactly the randomly generated bootstrap
// target names this suite creates; adoption never destroys unrelated
// (e.g. application) targets.
var bootstrapTargetPattern = regexp.MustCompile(`^w169-ci-bootstrap-[0-9a-f]{10}$`)

// bootstrapTargetSuffix returns a short random hex suffix so the shared trust
// plane helper configures a bootstrap target that cannot collide with a
// contaminated or pre-existing Core on a fixed name; each process gets its own
// `w169-ci-bootstrap-<suffix>` isolated Core.
func bootstrapTargetSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000"
	}
	return hex.EncodeToString(b[:])
}

func TestIntegrationRuntimeProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime string
		want    string
	}{
		{name: "default-docker", runtime: "", want: "docker"},
		{name: "explicit-docker", runtime: "docker", want: "docker"},
		{name: "explicit-podman", runtime: "podman", want: "podman"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASEHARBOR_TEST_RUNTIME", tc.runtime)
			if got := integrationRuntimeProvider(t); got != tc.want {
				t.Fatalf("runtime provider selection: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMissingRuntimeIsolationCannotFallBackToBootstrap(t *testing.T) {
	const sentinel = "w169-isolation-required-regression"
	t.Setenv("BASEHARBOR_TARGET", sentinel)
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "w169-ci-bootstrap",
		Targets: map[string]deployment.TargetDefinition{
			"w169-ci-bootstrap": {Runtime: deployment.RuntimeDefinition{Provider: "docker"}},
		},
	}
	selected, err := selectedTargetName("", os.Getenv("BASEHARBOR_TARGET"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if selected != sentinel {
		t.Fatalf("missing isolation selected %q instead of guard %q", selected, sentinel)
	}
	if _, err := cfg.ResolveTarget(selected, ""); err == nil {
		t.Fatal("unconfigured isolation guard resolved to the bootstrap target")
	}
}
