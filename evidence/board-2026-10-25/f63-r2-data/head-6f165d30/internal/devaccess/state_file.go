package devaccess

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

const maxDeveloperStateBytes = 16 << 10

// The existing Target files remain the authority. Do not follow redirected
// parents or a predictable temporary file when preserving/changing that state.
func checkDeveloperStateParents(path string) error {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("developer access state parent is not a regular directory")
		}
		if filepath.Dir(dir) == dir {
			return nil
		}
	}
}

func inspectDeveloperStateFile(path string) (os.FileInfo, error) {
	if err := checkDeveloperStateParents(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxDeveloperStateBytes {
		return nil, errors.New("developer access state must be a bounded protected regular file")
	}
	return info, nil
}

func readDeveloperStateFile(path string) ([]byte, error) {
	before, err := inspectDeveloperStateFile(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("developer access state changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDeveloperStateBytes+1))
	if err != nil {
		return nil, err
	}
	after, err := inspectDeveloperStateFile(path)
	if err != nil || !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || len(data) > maxDeveloperStateBytes {
		return nil, errors.New("developer access state changed during read")
	}
	return data, nil
}

func writeDeveloperStateFile(path string, data []byte) error {
	if len(data) > maxDeveloperStateBytes {
		return errors.New("developer access state exceeds its size bound")
	}
	before, err := inspectDeveloperStateFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := checkDeveloperStateParents(path); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".developer-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	after, err := inspectDeveloperStateFile(path)
	if before == nil {
		if !errors.Is(err, os.ErrNotExist) {
			return errors.New("developer access state appeared during change; retry from its actual state")
		}
	} else if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("developer access state changed before replacement")
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
