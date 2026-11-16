package main

import (
	"os"
	"path/filepath"
)

// localRuntimeOwnerStatErr reports os.Stat for the retained local runtime owner
// at <root>/targets/<target>/runtime. It is a package variable seam so the
// "only os.ErrNotExist proves absence" decision in discoverFullDestroyTargets
// can be tested with an injected non-ENOENT failure instead of filesystem
// permissions, which differ on privileged CI runners.
var localRuntimeOwnerStatErr = func(root, target string) error {
	_, err := os.Stat(filepath.Join(root, "targets", target, "runtime"))
	return err
}
