package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// TestSharedConsumersSerializeOnTargetOwner is the shared-consumer conflict
// cell: two consumers entering the real production lifecycle lock on one
// Target concurrently cannot both pass; the second observes the active-owner
// error instead of running in parallel, and is admitted once the first done.
func TestSharedConsumersSerializeOnTargetOwner(t *testing.T) {
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	hold := func() error {
		unlock, err := lockApplicationTargetLifecycle(resolvedApplication{Target: target, TargetStateRoot: root})
		if err != nil {
			return err
		}
		defer unlock()
		time.Sleep(150 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup
	firstDone := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		firstDone <- hold()
	}()
	time.Sleep(40 * time.Millisecond)
	concurrent := hold()
	if concurrent == nil || !strings.Contains(concurrent.Error(), "Target application lifecycle is active") {
		wg.Wait()
		t.Fatalf("second consumer entered the Target owner concurrently (err=%v); shared changes are not serialized", concurrent)
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("first production consumer failed: %v", err)
	}
	wg.Wait()
	// Once the first consumer completed, the same production entry admits it.
	if err := waitWhileBlocked(hold, "Target application lifecycle is active", 2*time.Second); err != nil {
		t.Fatalf("Target owner unavailable after the first consumer completed: %v", err)
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
			Target:          deployment.ResolvedTarget{Name: "docker-dev", RuntimeProvider: "docker", AccessProvider: "local", AccessReference: "local", Scope: "local"},
			TargetStateRoot: root,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "child rejected: %v\n", err)
			os.Exit(3)
		}
		unlock()
		fmt.Fprintln(os.Stderr, "child entered the Target owner while another process held it")
		os.Exit(1)
	}
	target := configureTestTarget(t)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := lockApplicationTargetLifecycle(resolvedApplication{Target: target, TargetStateRoot: root})
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
