package application

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Keep the directory mounted by running consumers. Backend reconciliation owns
// its children, while selected Identity material has a separate provider owner.
func resetWorkloadBindingProjection(m Manifest, root string) error {
	if err := checkPrivateRuntimeOwnerParents(root); err != nil {
		return fmt.Errorf("workload binding projection parent is redirected or unavailable")
	}
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
		return os.Chmod(root, 0755)
	}
	if err != nil || !ownedWorkloadProjectionDirectory(info) {
		return fmt.Errorf("workload binding projection must be an existing regular directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	// Check independent material before changing any backend projection.
	for _, entry := range entries {
		if entry.Name() == IdentityBindingName && HasIdentity(m) {
			info, err := os.Lstat(filepath.Join(root, entry.Name()))
			if err != nil || !ownedWorkloadProjectionDirectory(info) {
				return fmt.Errorf("selected Identity projection must be an owned regular directory")
			}
		}
	}
	for _, entry := range entries {
		if entry.Name() == IdentityBindingName && HasIdentity(m) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	// This directory is the read-only bind-mount root exposed to the selected
	// workload. MkdirAll's requested mode is reduced by umask, so normalize it
	// explicitly after validating ownership and removing stale projections.
	return os.Chmod(root, 0755)
}

func ownedWorkloadProjectionDirectory(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Geteuid())
}

// verifyConsumerReadableProjection checks access using the mode bits that an
// arbitrary non-root UID receives, rather than the developer/Core owner's
// access. The projection is bind-mounted read-only into the authorized app.
func verifyConsumerReadableProjection(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("workload service binding projection is not traversable")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workload service binding projection contains a symbolic link")
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("workload service binding projection metadata is unavailable")
		}
		mode := info.Mode().Perm()
		if info.IsDir() {
			if mode&0o005 != 0o005 || mode&0o022 != 0 {
				return fmt.Errorf("workload service binding directory is not readable/traversable by an arbitrary UID")
			}
			return nil
		}
		if !info.Mode().IsRegular() || mode&0o004 == 0 || mode&0o022 != 0 {
			return fmt.Errorf("workload service binding file is not safely readable by an arbitrary UID")
		}
		return nil
	})
}
