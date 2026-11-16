package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// Production-path binding cells for t-20261018-r27-mutation-lock (RUNDE 1).
//
// Card outcome: all clients use the existing owner lock; application AND
// provider changes are bound to the current revision and the exact consent;
// shared consumers conflict correctly; conflicting plans require re-preview.
//
// Every cell enters through a real production lifecycle function (destroy
// authorization, shared reconcile, selection mutation, Target lifecycle
// entry), not through a helper unit, so a bypass around the seam fails the
// cells, not just a helper test.

func nonInteractiveContext(ctx context.Context) context.Context {
	return cli.WithOutputOptions(ctx, cli.OutputOptions{NonInteractive: true})
}

// waitWhileBlocked polls fn until it stops returning the "already active"
// contention error or the deadline passes; returns the last error.
func waitWhileBlocked(fn func() error, contains string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	var last error
	for {
		last = fn()
		if last == nil || !strings.Contains(last.Error(), contains) {
			return last
		}
		if time.Now().After(deadline) {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestStaleDestructionConsentRequiresRepreview is the destroy stale-consent
// cell: the production authorization path must reject a consent bound to an
// outdated consequence plan and must expose the fresh plan for re-preview.
func TestStaleDestructionConsentRequiresRepreview(t *testing.T) {
	configureTestTarget(t)
	repo := t.TempDir()
	repoManifest := filepath.Join(repo, application.RepositoryManifestName)
	m := application.WithWorkloadComponents(application.New("lock-stale-consent", "dev", false, false, false), "lock-stale-app")
	mustWriteInitializedAuthoredManifest(t, repoManifest, m)
	t.Chdir(repo)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "destroy")
	if err != nil {
		t.Fatalf("resolve destroy candidate: %v", err)
	}
	if _, err := recordPendingDeployment(context.Background(), resolved); err != nil {
		t.Fatalf("record initial deployment state: %v", err)
	}
	resolved, err = resolveApplication(context.Background(), application.DefaultStore(), nil, "destroy")
	if err != nil {
		t.Fatal(err)
	}

	// Preview: the consequence plan the operator acknowledges.
	plan, err := buildApplicationDestructionPlan(context.Background(), resolved, true)
	if err != nil {
		t.Fatal(err)
	}

	// The authoritative deployment state changes AFTER the preview (a
	// conflicting plan the operator never acknowledged).
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	record.Observed.State = "applying"
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}

	// The production authorization path must refuse the outdated consent and
	// must NOT fall back to prompting (non-interactive: fail closed).
	ctx := withProviderChangeConsent(nonInteractiveContext(context.Background()), &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID})
	current, authErr := authorizeApplicationDestruction(ctx, resolved, true, &bytes.Buffer{})
	if authErr == nil {
		t.Fatal("stale destruction consent admitted; production path must require re-preview")
	}
	if !strings.Contains(authErr.Error(), "stale-impact-consent") {
		t.Fatalf("stale destruction consent refused with the wrong cause: %v", authErr)
	}
	if current.Scope.PlanID == plan.Scope.PlanID {
		t.Fatal("destruction plan does not bind the current revision; a stale consent could never be detected")
	}

	// Acknowledging with the FRESH plan id must pass the consent gate (the
	// refusal above proved staleness binding, not a policy veto).
	okCtx := withProviderChangeConsent(nonInteractiveContext(context.Background()), &application.ProviderChangeConsent{PlanID: current.Scope.PlanID})
	if _, err := authorizeApplicationDestruction(okCtx, resolved, true, &bytes.Buffer{}); err != nil && strings.Contains(err.Error(), "stale-impact-consent") {
		t.Fatalf("current-plan consent refused as stale: %v", err)
	}
}

// TestSharedBackendReconcileRejectsSecondProcess is the shared-consumer cell
// across real processes: while one process holds the shared provider owner,
// a second separate process entering the production reconcile path must be
// rejected fail-closed with the explicit contention message.
func TestSharedBackendReconcileRejectsSecondProcess(t *testing.T) {
	if os.Getenv("BAHA_TEST_SHARED_CHILD") == "reconcile" {
		m := application.New("shared-a", "dev", true, false, false)
		m.ApplicationID = application.MustNewApplicationID()
		m = application.WithSQLInstances(m, "primary")
		dir := filepath.Join(os.Getenv("BAHA_TEST_SHARED_DIR"), "providers", "shared-backends", "core")
		if err := os.MkdirAll(dir, 0700); err != nil {
			fmt.Fprintln(os.Stderr, "child mkdir failed")
			os.Exit(4)
		}
		_, err := application.ReconcileSharedBackends(context.Background(), nil, nil, os.Getenv("BAHA_TEST_SHARED_DIR"), "shared-a", m, application.RuntimeFiles{})
		if err != nil && strings.Contains(err.Error(), "shared provider lifecycle is already active") {
			// Expected fail-closed rejection of the concurrent shared writer.
			os.Exit(3)
		}
		if err == nil {
			fmt.Fprintln(os.Stderr, "child entered shared reconcile while another process held it")
		} else {
			fmt.Fprintf(os.Stderr, "child reconcile failed for the wrong reason: %v\n", err)
		}
		os.Exit(1)
	}
	dir := t.TempDir()
	sharedDir := filepath.Join(dir, "providers", "shared-backends", "core")
	if err := os.MkdirAll(sharedDir, 0700); err != nil {
		t.Fatal(err)
	}
	parentUnlock, err := coreinstallation.AcquireLifecycleLock(sharedDir)
	if err != nil {
		t.Fatalf("parent shared lock setup: %v", err)
	}
	t.Cleanup(parentUnlock)
	child := exec.Command(os.Args[0], "-test.run=^TestSharedBackendReconcileRejectsSecondProcess$")
	child.Env = append(os.Environ(), "BAHA_TEST_SHARED_CHILD=reconcile", "BAHA_TEST_SHARED_DIR="+dir)
	out, err := child.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		t.Fatalf("second process was not rejected fail-closed on the shared provider owner; exit=%v output:\n%s", err, out)
	}
}

// TestTargetSelectionMutationRejectsSecondProcess proves the Target selection
// owner serialization across two real processes: while one process holds the
// selection owner through the production mutation path, a second separate
// process must be rejected.
func TestTargetSelectionMutationRejectsSecondProcess(t *testing.T) {
	if os.Getenv("BAHA_TEST_SELECTION_CHILD") == "1" {
		os.Setenv("XDG_CONFIG_HOME", os.Getenv("BAHA_TEST_SELECTION_CONFIG"))
		os.Setenv("XDG_DATA_HOME", os.Getenv("BAHA_TEST_SELECTION_DATA"))
		if err := mutatePersistedTarget("docker-dev", false); err != nil {
			fmt.Fprintf(os.Stderr, "child selection mutation rejected: %v\n", err)
			os.Exit(3)
		}
		fmt.Fprintln(os.Stderr, "child entered the selection owner while another process held it")
		os.Exit(1)
	}
	configureTestTarget(t)
	unlock, err := lockTargetSelectionOwner()
	if err != nil {
		t.Fatalf("parent selection lock setup: %v", err)
	}
	t.Cleanup(unlock)
	child := exec.Command(os.Args[0], "-test.run=^TestTargetSelectionMutationRejectsSecondProcess$")
	child.Env = append(os.Environ(),
		"BAHA_TEST_SELECTION_CHILD=1",
		"BAHA_TEST_SELECTION_CONFIG="+os.Getenv("XDG_CONFIG_HOME"),
		"BAHA_TEST_SELECTION_DATA="+os.Getenv("XDG_DATA_HOME"),
	)
	out, err := child.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		t.Fatalf("second process entered the Target selection owner concurrently; exit=%v output:\n%s", err, out)
	}
}

// TestSharedConsumersSerializeOnAppOwner is the per-app critical-section cell:
// two writers to the SAME application on one Target never hold the lock at the
// same instant (the critical section is mutually exclusive), and the second is
// admitted once the first releases. The CLI retries on contention (bounded,
// destroy-priority), so the assertion is mutual exclusion of the critical
// section, not an immediate fail-fast: two overlapping holders must not coexist.
func TestSharedConsumersSerializeOnAppOwner(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	scope := "shared-app"

	var held int32
	var maxConcurrent int32
	count := func() {
		n := atomic.AddInt32(&held, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if n <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, n) {
				break
			}
		}
	}
	release := func() { _ = atomic.AddInt32(&held, -1) }

	const iters = 25
	done := make(chan error, iters)
	for i := 0; i < iters; i++ {
		go func() {
			unlock, herr := lockApplicationTargetLifecycle(resolvedApplication{
				Target:             target,
				TargetStateRoot:    root,
				DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: scope},
				Manifest:           application.New(scope, "dev", false, false, false),
			}, false)
			if herr != nil {
				done <- herr
				return
			}
			count()
			time.Sleep(5 * time.Millisecond)
			release()
			unlock()
			done <- nil
		}()
	}
	for i := 0; i < iters; i++ {
		if err := <-done; err != nil {
			t.Fatalf("same-app writer failed to acquire (retry loop): %v", err)
		}
	}
	if maxConcurrent > 1 {
		t.Fatalf("same-app critical section not mutually exclusive: maxConcurrent=%d", maxConcurrent)
	}
}

// TestProductionTargetWideMutationCannotOverlapLocalAppLifecycle is the F3
// production regression in direction 1: a local app holds its per-app lock and a
// REAL target-wide lifecycle mutation through the production seam
// (lockApplicationTargetLifecycle, the remote/destroy/stop else-branch that now
// routes through coreinstallation.AcquireTargetWideLifecycleLock) must fail
// closed — it must not begin while the app lifecycle is held.
func TestProductionTargetWideMutationCannotOverlapLocalAppLifecycle(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// A local app holds its per-app lock through the production per-app path.
	appUnlock, aerr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-lived"},
		Manifest:           application.New("app-lived", "dev", false, false, false),
	}, false)
	if aerr != nil {
		t.Fatalf("local app acquire: %v", aerr)
	}
	t.Cleanup(appUnlock)

	// A real target-wide mutation through the production seam (destroy/remote
	// else-branch) must fail closed while the app lifecycle is held.
	wideUnlock, werr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{}, // no app id => target-wide
		Manifest:           application.New("ghost", "dev", false, false, false),
	}, true)
	if werr == nil {
		wideUnlock()
		t.Fatal("target-wide mutation began while a local app lifecycle was held")
	}
}

// TestProductionAppCannotBeginDuringTargetWideScan is the F3 production
// regression in the opposite direction: while the production target-wide path
// holds .installation.lock to run its scan, a concurrent local app must not be
// able to create its per-app dotfile (it fails on the held target-wide lock).
func TestProductionAppCannotBeginDuringTargetWideScan(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// The target-wide mutation holds .installation.lock (its scan window).
	wideUnlock, werr := coreinstallation.AcquireTargetWideLifecycleLock(root)
	if werr != nil {
		t.Fatalf("target-wide acquire: %v", werr)
	}
	// While the scan holds the lock, a local app cannot begin its lifecycle.
	// The per-app primitive AcquireAppLifecycleLock takes the same
	// .installation.lock critical section and must fail fast on flock contention
	// (not begin during the held scan).
	appUnlock, aerr := coreinstallation.AcquireAppLifecycleLock(root, "app-late")
	if aerr == nil {
		appUnlock()
		t.Fatal("a local app began its lifecycle during a held target-wide scan")
	}

	// Once the scan releases, the app can begin normally (the critical section
	// is open again and no dotfile is held).
	wideUnlock()
	appUnlock, aerr = coreinstallation.AcquireAppLifecycleLock(root, "app-late")
	if aerr != nil {
		t.Fatalf("local app could not begin after the target-wide scan released: %v", aerr)
	}
	appUnlock()
}

// TestProductionTargetWideScanFailsClosedOnScanError is the F3 fail-closed
// requirement: a target-wide scan over a corrupt/unreadable deployments namespace
// must fail closed, never treat the error as an empty scan. This verifies the
// production AcquireTargetWideLifecycleLock path directly.
func TestProductionTargetWideScanFailsClosedOnScanError(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Create a deployments dir we can make unreadable (non-root only). If the
	// process is root, skip: root bypasses permission checks and the failure
	// cannot be reproduced.
	deploymentsDir := filepath.Join(root, "deployments")
	if err := os.MkdirAll(deploymentsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission checks; cannot reproduce a scan error")
	}
	if err := os.Chmod(deploymentsDir, 0000); err != nil {
		t.Skipf("cannot chmod deployments dir: %v", err)
	}
	defer os.Chmod(deploymentsDir, 0700)

	if _, serr := coreinstallation.AcquireTargetWideLifecycleLock(root); serr == nil {
		t.Fatal("target-wide scan did not fail closed over an unreadable deployments namespace")
	}
}

// TestIndependentAppsDoNotSerializeOnSharedTarget is the per-app scope cell:
// two INDEPENDENT service-less applications that merely share one Target run in
// parallel. This PROVES the serialization wall is per-app, not per-target:
// beta's acquisition signal must fire while alpha is still CONFIRMED holding its
// own per-app lock, under a deadline shorter than alpha's hold. If both IDs were
// routed through one target-wide lock, beta would block until alpha released and
// this assertion would fail.
func TestIndependentAppsDoNotSerializeOnSharedTarget(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// alpha holds its own per-app lock until cleanup releases it explicitly.
	alphaUnlock, aerr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "alpha"},
		Manifest:           application.New("alpha", "dev", false, false, false),
	}, false)
	if aerr != nil {
		t.Fatalf("alpha holder failed to acquire: %v", aerr)
	}
	t.Cleanup(alphaUnlock)

	// beta acquires in a goroutine; its acquisition must complete while alpha is
	// still held (alpha is released only by cleanup after this test body).
	betaAcquired := make(chan struct{})
	go func() {
		betaUnlock, berr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "beta"},
			Manifest:           application.New("beta", "dev", false, false, false),
		}, false)
		if berr != nil {
			t.Errorf("independent app beta was blocked by a DIFFERENT app on the same target (err=%v); independent apps must not serialize", berr)
			close(betaAcquired)
			return
		}
		betaUnlock()
		close(betaAcquired)
	}()

	// beta must be admitted WHILE alpha is still held: the acquire signal fires
	// under a short deadline that is far shorter than any serial hand-off would
	// allow. If beta had blocked on a target-wide lock until alpha released,
	// this would time out and fail.
	select {
	case <-betaAcquired:
	case <-time.After(3 * time.Second):
		t.Fatal("beta did not acquire while alpha still held its per-app lock; independent apps serialized by a target-wide lock")
	}
}

// TestDestroyNotStarvedByBoundedRetry proves fairness under the SAME per-app
// key: a long-lived up-holder on deployment ID "app" and a destroy on the SAME
// ID contend for one per-app lock file. The destroy (allowPriority) must receive
// a real contention result (a held lock, not an immediate grant) BEFORE the
// holder releases, then win within the documented finite bound. This proves the
// bounded, destroy-priority policy, not merely that destroy and up use different
// keys.
func TestDestroyNotStarvedByBoundedRetry(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// Long-lived up-holder on the SAME deployment ID the destroy will target.
	holderDone := make(chan struct{})
	go func() {
		unlock, herr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app"},
			Manifest:           application.New("app", "dev", false, false, false),
		}, false)
		if herr != nil {
			t.Errorf("up holder failed: %v", herr)
			close(holderDone)
			return
		}
		time.Sleep(300 * time.Millisecond)
		unlock()
		close(holderDone)
	}()

	// Let the holder establish the lock before the destroy contends.
	select {
	case <-holderDone:
		t.Fatal("holder exited before destroy started")
	case <-time.After(120 * time.Millisecond):
	}

	// The destroy contends on the SAME per-app key in a goroutine and reports
	// whether it observed a held lock (contention) before the holder released.
	destroyContended := make(chan bool, 1)
	destroyAcquired := make(chan struct{})
	go func() {
		// Probe the per-app lock directly: while the holder still owns it, the
		// non-blocking acquisition returns a contention error. This proves the
		// destroy contends for the SAME per-app key and observes the held lock.
		contented := false
		if _, perr := coreinstallation.AcquireAppLifecycleLock(root, "app"); perr != nil {
			contented = true
		}
		destroyContended <- contented

		start := time.Now()
		destroyUnlock, derr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app"},
			Manifest:           application.New("app", "dev", false, false, false),
		}, true) // destroy takes priority under the bounded policy
		elapsed := time.Since(start)
		if derr != nil {
			t.Errorf("destroy failed to acquire within the bounded window (err=%v, elapsed=%v)", derr, elapsed)
			close(destroyAcquired)
			return
		}
		if elapsed > 5*time.Second {
			t.Errorf("destroy took too long (%v) after holder release; fairness bound violated", elapsed)
		}
		destroyUnlock()
		close(destroyAcquired)
	}()

	// While the destroy contends, the holder is still running (releases at ~300ms,
	// we are at ~120ms). The destroy must observe a held lock: it cannot acquire
	// the same per-app lock the holder still owns. Wait past the holder release so
	// the destroy can win within the finite bound, and confirm it was contented.
	select {
	case contented := <-destroyContended:
		if !contented {
			t.Fatal("destroy did not observe contention on the same per-app key")
		}
	case <-time.After(600 * time.Millisecond):
	}
	select {
	case <-destroyAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("destroy never acquired within the bounded window")
	}
}

// TestTargetLifecycleLockRejectsSecondProcess is the two-process cell on the
// application/Target production entry itself: a second OS process entering
// the same Target lifecycle seam must be rejected while the first holds it.
func TestTargetLifecycleLockRejectsSecondProcess(t *testing.T) {
	if os.Getenv("BAHA_TEST_TARGET_LOCK_CHILD") == "1" {
		os.Setenv("XDG_CONFIG_HOME", os.Getenv("BAHA_TEST_TARGET_LOCK_CONFIG"))
		os.Setenv("XDG_DATA_HOME", os.Getenv("BAHA_TEST_TARGET_LOCK_DATA"))
		root, err := deployment.TargetStateRoot("docker-dev")
		if err != nil {
			fmt.Fprintf(os.Stderr, "child target root: %v\n", err)
			os.Exit(4)
		}
		if err := os.MkdirAll(root, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "child mkdir: %v\n", err)
			os.Exit(4)
		}
		unlock, err := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             deployment.ResolvedTarget{Name: "docker-dev", RuntimeProvider: "docker", AccessProvider: "local", AccessReference: "local", Scope: "local"},
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-one"},
			Manifest:           application.New("app-one", "dev", false, false, false),
			TargetStateRoot:    root,
		}, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "child rejected: %v\n", err)
			os.Exit(3)
		}
		unlock()
		fmt.Fprintln(os.Stderr, "child entered the app owner while another process held it")
		os.Exit(1)
	}
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-one"},
		Manifest:           application.New("app-one", "dev", false, false, false),
	}, false)
	if err != nil {
		t.Fatalf("parent production entry: %v", err)
	}
	t.Cleanup(unlock)
	child := exec.Command(os.Args[0], "-test.run=^TestTargetLifecycleLockRejectsSecondProcess$")
	child.Env = append(os.Environ(),
		"BAHA_TEST_TARGET_LOCK_CHILD=1",
		"BAHA_TEST_TARGET_LOCK_CONFIG="+os.Getenv("XDG_CONFIG_HOME"),
		"BAHA_TEST_TARGET_LOCK_DATA="+os.Getenv("XDG_DATA_HOME"),
	)
	out, err := child.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 {
		t.Fatalf("second process entered the Target lifecycle owner concurrently; exit=%v output:\n%s", err, out)
	}
}

// TestUnenforcingFilesystemFailsClosedWithoutRetry is the command-path
// regression for 0427-179 r2: the filesystem-does-not-enforce condition is a
// classifiable remediation failure, not ordinary lock contention. When the
// per-app acquire routes through the production retry seam
// (lockApplicationTargetLifecycle), a filesystem that cannot enforce the
// cross-process flock must fail CLOSED IMMEDIATELY with the actionable
// filesystem remediation text, never enter the 15-second bounded retry loop.
// The probe is stubbed to FREE (a child that acquires proves enforcement is
// dropped) so the production AcquireAppLifecycleLock returns
// ErrLockFilesystemNotEnforcing without a second real OS process. A fast
// return under a sub-second deadline proves the retry loop is bypassed; the
// 15s retry would otherwise elapse and hide the cause behind the generic
// acquisition-deadline message. Fairness (destroy-priority) and the bounded
// retry remain intact for genuine held-lock contention.
func TestUnenforcingFilesystemFailsClosedWithoutRetry(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	origProbe := stableid.LockProbeFunc
	stableid.LockProbeFunc = func(string) (bool, error) { return true, nil }
	defer func() { stableid.LockProbeFunc = origProbe }()

	start := time.Now()
	_, cerr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-unenforcing"},
		Manifest:           application.New("app-unenforcing", "dev", false, false, false),
	}, false)
	elapsed := time.Since(start)

	if cerr == nil {
		t.Fatalf("acquire succeeded while the filesystem cannot enforce the lock cross-process; mutual exclusion is advisory-only")
	}
	if !strings.Contains(cerr.Error(), "does not enforce the lifecycle lock") {
		t.Fatalf("want the actionable filesystem remediation, got: %v", cerr)
	}
	if strings.Contains(cerr.Error(), "acquisition deadline of 15s") {
		t.Fatalf("the not-enforcing failure was masked by the 15s contention retry: %v", cerr)
	}
	if elapsed > time.Second {
		t.Fatalf("acquire took %v on a not-enforcing filesystem; the bounded 15s retry loop must be bypassed for immediate fail-closed", elapsed)
	}
}

// TestGenuineContentionStillEntersBoundedRetry proves the command-path fix does
// not discard the bounded, destroy-priority retry for genuine flock contention.
// While a holder keeps the per-app lock, a destroy (allowPriority) must observe
// a real held-lock contention result and win within the finite bound; the
// not-enforcing fix must not collapse this path. The holder is created with the
// probe stubbed to HELD (correctly enforcing) so the acquire succeeds and holds.
func TestGenuineContentionStillEntersBoundedRetry(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	origProbe := stableid.LockProbeFunc
	stableid.LockProbeFunc = func(string) (bool, error) { return false, nil }
	defer func() { stableid.LockProbeFunc = origProbe }()

	holderDone := make(chan struct{})
	go func() {
		unlock, herr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-contended"},
			Manifest:           application.New("app-contended", "dev", false, false, false),
		}, false)
		if herr != nil {
			t.Errorf("up holder failed: %v", herr)
			close(holderDone)
			return
		}
		time.Sleep(300 * time.Millisecond)
		unlock()
		close(holderDone)
	}()

	select {
	case <-holderDone:
		t.Fatal("holder exited before destroy started")
	case <-time.After(120 * time.Millisecond):
	}

	destroyAcquired := make(chan struct{})
	go func() {
		start := time.Now()
		destroyUnlock, derr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "app-contended"},
			Manifest:           application.New("app-contended", "dev", false, false, false),
		}, true)
		elapsed := time.Since(start)
		if derr != nil {
			t.Errorf("destroy failed to acquire within the bounded window (err=%v): %v", derr, elapsed)
			close(destroyAcquired)
			return
		}
		if elapsed > 5*time.Second {
			destroyUnlock()
			t.Errorf("destroy took too long (%v) after holder release; bounded retry bound violated", elapsed)
			close(destroyAcquired)
			return
		}
		destroyUnlock()
		close(destroyAcquired)
	}()

	select {
	case <-destroyAcquired:
	case <-time.After(5 * time.Second):
		t.Fatal("destroy never acquired within the bounded window")
	}
}
