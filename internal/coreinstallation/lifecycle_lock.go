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
