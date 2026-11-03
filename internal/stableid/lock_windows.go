//go:build windows

package stableid

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Installation lock path is the shared, provider-neutral kernel lock file used
// across a single installation state root. It lives in stableid so that the
// lock primitive is a leaf dependency: coreinstallation and serviceaccess both
// depend on it without importing each other, which keeps the dependency graph
// acyclic.

func lock(root string) (func(), error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("Core lifecycle lock requires a private state directory")
	}
	file, err := os.OpenFile(filepath.Join(root, ".installation.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	if err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped); _ = file.Close() }, nil
}
