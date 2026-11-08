//go:build !windows

package stableid

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// lockNamed opens the named kernel lock file under root and takes an exclusive,
// non-blocking advisory lock. The lifecycle lock and the bootstrap lock use
// DIFFERENT files (see lifecycleLockFile / bootstrapLockFile) so that bootstrap
// and provider upgrades of the same root never contend with each other.
func lockNamed(root string, name string) (func(), error) {
	fd, err := unix.Open(filepath.Join(root, name), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = file.Close() }, nil
}

func lock(root string) (func(), error) {
	return lockNamed(root, lifecycleLockFile)
}

func bootstrapLock(root string) (func(), error) {
	return lockNamed(root, bootstrapLockFile)
}
