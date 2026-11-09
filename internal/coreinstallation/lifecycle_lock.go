package coreinstallation

import "github.com/mcpdev80/baseharbor/internal/stableid"

// This file previously contained the lifecycle-lock primitives. They have been
// extracted to internal/stableid (a stdlib-only leaf) so that coreinstallation
// can import serviceaccess without creating an import cycle: serviceaccess
// imports stableid (not coreinstallation) and coreinstallation imports
// serviceaccess. The lock contract, error messages and behaviour are unchanged.

// AcquireLifecycleLock re-exports the stableid lifecycle lock with the same
// signature callers across the product expect. See internal/stableid for the
// authoritative implementation and the reason for the move.
func AcquireLifecycleLock(root string) (func(), error) {
	return stableid.AcquireLifecycleLock(root)
}

// AcquireStateFileLock re-exports the stableid state-file lock with the same
// signature callers across the product expect.
func AcquireStateFileLock(root string) (func(), error) {
	return stableid.AcquireStateFileLock(root)
}

// AcquireBootstrapLock re-exports the stableid bootstrap lock — a SEPARATE
// kernel lock from the lifecycle lock — so bootstrap of a fresh durable state
// root is serialised and exactly one process ever owns the new authority before
// the management host binds.
func AcquireBootstrapLock(root string) (func(), error) {
	return stableid.AcquireBootstrapLock(root)
}

// AcquireAppLifecycleLock re-exports the stableid per-app lifecycle lock with
// the same signature callers across the product expect. See internal/stableid
// for the authoritative implementation: the lock is keyed by the application's
// OWN deployments directory (root/deployments/<scope>) so two independent
// applications that share a host Target do not serialize each other, while two
// writers to the same application still do. The Target as a whole is protected
// by an independent owner lock (AcquireLifecycleLock), not by this per-app lock.
func AcquireAppLifecycleLock(root string, scope string) (func(), error) {
	return stableid.AcquireAppLifecycleLock(root, scope)
}
