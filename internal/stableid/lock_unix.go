//go:build !windows

package stableid

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
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
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			message := lifecycleLockHolderMessage(file, name)
			_ = file.Close()
			return nil, fmt.Errorf("%s: %w", message, err)
		}
		_ = file.Close()
		return nil, err
	}
	if err := file.Truncate(0); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	if _, err := fmt.Fprintln(file, os.Getpid()); err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	return func() { _ = file.Truncate(0); _ = unix.Flock(fd, unix.LOCK_UN); _ = file.Close() }, nil
}

func lock(root string) (func(), error) {
	return lockNamed(root, lifecycleLockFile)
}

func bootstrapLock(root string) (func(), error) {
	return lockNamed(root, bootstrapLockFile)
}

// This diagnostic reads only the private lock file. A PID is advisory: do not
// signal a process automatically, because PID reuse and exited holders exist.
func lifecycleLockHolderMessage(file *os.File, name string) string {
	base := "lock " + name + " is held; inspect the owner process before retrying; do not force-remove the lock file"
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return base
	}
	buf := make([]byte, 64)
	n, _ := file.Read(buf)
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil || pid <= 0 {
		return base
	}
	state := "unknown"
	switch err := unix.Kill(pid, 0); {
	case err == nil:
		state = "alive"
	case errors.Is(err, unix.ESRCH):
		state = "not-running (possibly stale PID)"
	case errors.Is(err, unix.EPERM):
		state = "running (permission denied)"
	}
	return fmt.Sprintf("lock %s held by reported pid=%d state=%s; inspect with 'ps -p %d -o pid,stat,cmd' and stop it explicitly only after verifying ownership; retry afterward (never delete the lock file while held)", name, pid, state, pid)
}
