package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// lockApplicationTargetLifecycle serialises lifecycle mutation of a SINGLE
// application while computing and applying the consumer union. The existing
// lifecycle lock is not a new state owner.
//
// Scope (card 0427-176): the lock is now per-APPLICATION, keyed by the
// application's OWN deployments directory
// (<target-state-root>/deployments/<deployment-id>), not by the whole target.
// Two independent service-less applications that merely share a host target no
// longer serialize each other; two writers to the same application still do.
// The target as a whole is still protected by an independent owner lock
// (coreinstallation.AcquireLifecycleLock) that the core/local callers layer on
// top of this per-app lock.
//
// Fairness (card 0427-176): the kernel flock is FCFS and cannot be made
// priority, so the caller owns the acquisition policy. destroy/stop operations
// take a single immediate try first; every operation then retries with jittered
// exponential backoff up to a bounded deadline before failing closed with the
// same contention message. This bounds a destroy to finite-time acquisition so
// it cannot be starved by a continuous stream of ups on the same target.
//
// allowPriority lets destroy/stop win the first uncontended attempt before the
// bounded retry loop; up/apply wait for the first attempt.
func lockApplicationTargetLifecycle(resolved resolvedApplication, allowPriority bool) (func(), error) {
	if err := os.MkdirAll(resolved.TargetStateRoot, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(resolved.TargetStateRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("Target lifecycle state is unprotected or noncanonical")
	}

	// The per-app lock key is the application's deployment identity. For local
	// repository applications the lock is keyed inside the application's OWN
	// deployments namespace (<root>/deployments/.<id>.app_installation.lock), so
	// two independent apps on one target do not serialize each other. Remote
	// applications (no local deployment id, must not stage a local deployments
	// member) and control-plane operations (destroy/stop of the whole target,
	// which carry no application identity) keep the existing per-target
	// serialization via the target name, so neither stages a local deployments
	// member and control-plane stays strictly serialized per target.
	if resolved.DeploymentIdentity.DeploymentID != "" && !isRemoteApplication(resolved) {
		scope := resolved.DeploymentIdentity.DeploymentID
		unlock, err := coreinstallation.AcquireAppLifecycleLock(resolved.TargetStateRoot, scope)
		if err != nil {
			// An owner-missing or foreign root is a hard fail-closed condition, not
			// contention: surface it immediately so a Coreless/never-delivered app
			// is never stuck in the bounded retry loop. A filesystem that does NOT
			// enforce the lock across processes is a classifiable remediation
			// condition, not contention either: it never resolves by waiting, so
			// surface the actionable filesystem cause immediately instead of
			// retrying for the full 15s deadline and hiding it behind the generic
			// acquisition-deadline message. Only genuine flock contention (a
			// different writer holding the same app owner) enters the retry loop.
			if errors.Is(err, stableid.ErrLockFilesystemNotEnforcing) ||
				errors.Is(err, deployment.ErrDeploymentOwnerMissing) ||
				errors.Is(err, deployment.ErrDeploymentOwnerForeign) {
				return nil, err
			}
			return acquireLifecycleUnderRetry(nil, allowPriority, resolved.TargetStateRoot, scope)
		}
		return unlock, nil
	}

	// Target-wide whole-Target lifecycle mutation (remote/destroy/stop/control
	// plane): route through the target-wide scan path so the shared
	// .installation.lock is held for the whole mutation and held per-app dotfiles
	// are scanned. This fails closed while a local app lifecycle is active and
	// fails closed on a scan error — a target-wide mutation can never overlap a
	// local app lifecycle. The scan is not retryable contention: a held app is a
	// genuine conflict, so any failure here fails closed rather than entering the
	// bounded retry loop.
	unlock, err := coreinstallation.AcquireTargetWideLifecycleLock(resolved.TargetStateRoot)
	if err != nil {
		return nil, fmt.Errorf("Target application lifecycle is active: %w", err)
	}
	return unlock, nil
}

// acquireLifecycleUnderRetry implements the bounded, jittered, destroy-priority
// acquisition policy. allowPriority (destroy/stop) takes one immediate try
// before entering the shared retry loop; up/apply wait for the first attempt.
// The returned unlock is always non-nil on success.
func acquireLifecycleUnderRetry(initialUnlock func(), allowPriority bool, root string, scope string) (func(), error) {
	if initialUnlock != nil {
		return initialUnlock, nil
	}
	if allowPriority {
		unlock, err := coreinstallation.AcquireAppLifecycleLock(root, scope)
		if err == nil {
			return unlock, nil
		}
	}

	const (
		maxDelay = 500 * time.Millisecond
		total    = 15 * time.Second
	)
	start := time.Now()
	delay := 20 * time.Millisecond
	for {
		if time.Since(start) >= total {
			return nil, fmt.Errorf("Target application lifecycle is active: %w",
				errors.New("acquisition deadline of 15s exceeded while the target owner was busy"))
		}
		unlock, err := coreinstallation.AcquireAppLifecycleLock(root, scope)
		if err == nil {
			return unlock, nil
		}
		time.Sleep(delay)
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
		// Jitter prevents thundering-herd collapse when many writers converge.
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
	}
}
