//go:build !windows

package coreinstallation

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func lock(root string) (func(), error) {
	fd, err := unix.Open(filepath.Join(root, ".installation.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "installation-lock")
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = file.Close() }, nil
}
