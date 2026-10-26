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
		return os.MkdirAll(root, 0755)
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
	return nil
}

func ownedWorkloadProjectionDirectory(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Geteuid())
}
