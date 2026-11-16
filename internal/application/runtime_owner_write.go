package application

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func existingPrivateRuntimeOwner(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return nil, errors.New("private runtime owner must be an exclusively owned regular file")
	}
	return info, nil
}

// VerifyRuntimeFileOwner reports whether path is an exclusively owned regular
// runtime file (the ownership evidence the destroy/retire path derives a stop
// set from), or nil when it is missing. A present-but-unowned runtime file
// returns (nil, nil) — it is not an owned materialization and must never be
// treated as one. This backs the ownership-gated workload-stop path and the
// protected OTLP reader; the shared readWorkloadRuntimeEnv reader is kept
// strictly fail-closed (missing runtime.env is an error) so it cannot silently
// fail open for other workload callers.
func VerifyRuntimeFileOwner(path string) (bool, error) {
	owner, err := existingPrivateRuntimeOwner(path)
	if err != nil {
		return false, err
	}
	return owner != nil, nil
}

func writePrivateRuntimeOwnerFile(path string, content []byte) error {
	if err := checkPrivateRuntimeOwnerParents(path); err != nil {
		return err
	}
	before, err := existingPrivateRuntimeOwner(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".baha-private-owner-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	after, err := existingPrivateRuntimeOwner(path)
	if err != nil {
		return err
	}
	if (before == nil) != (after == nil) || (before != nil && !os.SameFile(before, after)) {
		return errors.New("private runtime owner changed during write")
	}
	return os.Rename(file.Name(), path)
}

func checkPrivateRuntimeOwnerParents(path string) error {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return err
	}
	for {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("private runtime owner parent must be a real directory")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}
