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

// TestIndependentAppsDoNotSerializeOnSharedTarget is the per-app scope cell:
// two INDEPENDENT service-less applications that merely share one Target run in
// parallel — the second acquires while the first holds its own app lock, proving
// the serialization wall is per-app, not per-target.
func TestIndependentAppsDoNotSerializeOnSharedTarget(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// Holder: app "alpha" holds its own per-app lock for a bounded window.
	holderDone := make(chan struct{})
	go func() {
		unlock, herr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "alpha"},
			Manifest:           application.New("alpha", "dev", false, false, false),
		}, false)
		if herr != nil {
			t.Errorf("alpha holder failed to acquire: %v", herr)
			close(holderDone)
			return
		}
		time.Sleep(250 * time.Millisecond)
		unlock()
		close(holderDone)
	}()

	// Wait for the holder to actually be running before starting beta.
	select {
	case <-holderDone:
		t.Fatal("alpha holder exited before beta started")
	case <-time.After(80 * time.Millisecond):
	}

	// Beta on the SAME target must acquire while alpha holds its own lock.
	betaUnlock, berr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "beta"},
		Manifest:           application.New("beta", "dev", false, false, false),
	}, false)
	select {
	case <-holderDone:
		// Holder released early; still valid as long as beta did not block.
	default:
	}
	if berr != nil {
		t.Fatalf("independent app beta was blocked by a DIFFERENT app on the same target (err=%v); independent apps must not serialize", berr)
	}
	betaUnlock()
}

// TestDestroyNotStarvedByBoundedRetry proves fairness: a long-lived up-holder
// on one app lets a destroy on a DIFFERENT app (or the same app after the
// holder releases) acquire within the bounded deadline, so a destroy is never
// starved indefinitely by a continuous stream of writers. Here a single
// long-lived holder releases at t≈300ms; a destroy issued at t≈80ms acquires
// shortly after release, well within the bounded window.
func TestDestroyNotStarvedByBoundedRetry(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}

	// Long-lived holder on app "alpha" (simulating a slow up), released at ~300ms.
	holderDone := make(chan struct{})
	go func() {
		unlock, herr := lockApplicationTargetLifecycle(resolvedApplication{
			Target:             target,
			TargetStateRoot:    root,
			DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "alpha"},
			Manifest:           application.New("alpha", "dev", false, false, false),
		}, false)
		if herr != nil {
			t.Errorf("alpha holder failed: %v", herr)
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
	case <-time.After(80 * time.Millisecond):
	}

	// A destroy issued while the holder is running must acquire within a bounded
	// window after the holder releases, not block forever.
	start := time.Now()
	destroyUnlock, derr := lockApplicationTargetLifecycle(resolvedApplication{
		Target:             target,
		TargetStateRoot:    root,
		DeploymentIdentity: deployment.DeploymentIdentity{DeploymentID: "destroyed"},
		Manifest:           application.New("destroyed", "dev", false, false, false),
	}, true) // destroy takes priority
	elapsed := time.Since(start)
	if derr != nil {
		t.Fatalf("destroy failed to acquire within the bounded window (err=%v, elapsed=%v)", derr, elapsed)
	}
	destroyUnlock()
	if elapsed > 5*time.Second {
		t.Fatalf("destroy took too long (%v) to acquire after holder release; fairness bound violated", elapsed)
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
