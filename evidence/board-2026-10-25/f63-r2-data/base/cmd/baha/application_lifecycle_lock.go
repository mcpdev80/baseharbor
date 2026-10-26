package main

import (
	"fmt"
	"os"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

// Serialize application mutations for a Target while computing and applying
// the consumer union. The existing lifecycle lock is not a new state owner.
func lockApplicationTargetLifecycle(resolved resolvedApplication) (func(), error) {
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
	unlock, err := coreinstallation.AcquireLifecycleLock(resolved.TargetStateRoot)
	if err != nil {
		return nil, fmt.Errorf("Target application lifecycle is active: %w", err)
	}
	return unlock, nil
}
