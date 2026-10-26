package externalprovider

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const maxRegistryBytes = 16 << 20

func checkStoreParents(path string) error {
	if path == "" || filepath.Clean(path) == "." {
		return errors.New("external provider registry path is required")
	}
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return err
	}
	for {
		info, err := os.Lstat(dir)
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("external provider registry parent is not a real directory")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

func checkStoreFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("external provider registry requires a protected regular owner file")
	}
	if err := checkStoreOwner(info); err != nil {
		return nil, err
	}
	return info, nil
}

func checkStoreOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return errors.New("external provider registry requires an exclusively owned file")
	}
	return nil
}

func readStoreFile(path string) ([]byte, error) {
	if err := checkStoreParents(path); err != nil {
		return nil, err
	}
	before, err := checkStoreFile(path)
	if err != nil {
		return nil, err
	}
	if before.Size() > maxRegistryBytes {
		return nil, errors.New("external provider registry exceeds size limit")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || checkStoreOwner(opened) != nil {
		return nil, errors.New("external provider registry owner changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRegistryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRegistryBytes {
		return nil, errors.New("external provider registry exceeds size limit")
	}
	after, err := checkStoreFile(path)
	if err != nil || !os.SameFile(before, after) {
		return nil, errors.New("external provider registry owner changed during inspection")
	}
	return data, nil
}

func writeStoreFile(path string, data []byte) error {
	if len(data) > maxRegistryBytes {
		return errors.New("external provider registry exceeds size limit")
	}
	if err := checkStoreParents(path); err != nil {
		return err
	}
	if _, err := checkStoreFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".external-provider-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := checkStoreParents(path); err != nil {
		return err
	}
	if _, err := checkStoreFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("sync external provider registry owner: %w", err)
	}
	defer dir.Close()
	return dir.Sync()
}

func openStoreLock(path string) (*os.File, error) {
	if err := checkStoreParents(path); err != nil {
		return nil, err
	}
	if _, err := checkStoreFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	after, inspectErr := checkStoreFile(path)
	if err != nil || inspectErr != nil || !os.SameFile(opened, after) || checkStoreOwner(opened) != nil {
		file.Close()
		return nil, errors.New("external provider registry lock owner changed")
	}
	return file, nil
}
