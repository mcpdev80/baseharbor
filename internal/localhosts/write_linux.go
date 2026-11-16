//go:build linux

package localhosts

import (
	"errors"
	"os"
	"syscall"
)

// ApplySystem is the deliberately narrow privileged helper: no caller-supplied
// pathname, no DNS/service changes, and an exact previous-block comparison.
func ApplySystem(change Change) error {
	if os.Geteuid() != 0 {
		return errors.New("hosts writer requires administrator privileges")
	}
	return applyFile("/etc/hosts", change)
}

func applyFile(path string, change Change) error {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 4*1024*1024 {
		return errors.New("unsupported hosts file")
	}
	body := make([]byte, info.Size())
	if len(body) > 0 {
		if _, err := file.ReadAt(body, 0); err != nil {
			return err
		}
	}
	next, err := Rewrite(string(body), change)
	if err != nil || next == string(body) {
		return err
	}
	// Keep the existing inode/mode/ownership, including hosts files mounted by the OS.
	if _, err := file.WriteAt([]byte(next), 0); err != nil {
		return err
	}
	if err := file.Truncate(int64(len(next))); err != nil {
		return err
	}
	return file.Sync()
}
