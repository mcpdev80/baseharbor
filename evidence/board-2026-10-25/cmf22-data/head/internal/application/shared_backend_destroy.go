package application

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SharedBackendDestroyPlan validates retained canonical state before selected-
// target destruction. Missing ownership state is not proof that consumers left.
func SharedBackendDestroyPlan(dataDir, namespace string) ([]SharedBackendFiles, error) {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "shared-backends")
	for _, directory := range []string{filepath.Dir(root), root} {
		info, err := os.Lstat(directory)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("shared backend ownership directory must be canonical: %s", directory)
		}
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var modules []SharedBackendFiles
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refuse shared backend destruction through symlink %s", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		files := SharedBackendFilesAt(dataDir, namespace, entry.Name())
		info, err := os.Lstat(files.State)
		if err != nil {
			return nil, fmt.Errorf("shared backend ownership state unavailable for %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("shared backend ownership state must be a regular file for %s", entry.Name())
		}
		state, err := readSharedBackendDestroyState(files.State, info)
		if err != nil {
			return nil, err
		}
		if state.Environment != entry.Name() {
			return nil, fmt.Errorf("shared backend environment ownership mismatch")
		}
		for key, app := range state.Applications {
			if len(app.SQL) > 0 || len(app.Cache) > 0 {
				return nil, fmt.Errorf("shared backend still contains application-owned resources for %s; destroy the application before its target", key)
			}
		}
		modules = append(modules, files)
	}
	return modules, nil
}

func readSharedBackendDestroyState(path string, expected os.FileInfo) (sharedBackendState, error) {
	file, err := os.Open(path)
	if err != nil {
		return sharedBackendState{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return sharedBackendState{}, err
	}
	const maximumStateBytes = 4 << 20
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) || opened.Mode().Perm()&0077 != 0 || opened.Size() > maximumStateBytes {
		return sharedBackendState{}, fmt.Errorf("shared backend ownership state is unprotected, replaced or oversized")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumStateBytes+1))
	if err != nil {
		return sharedBackendState{}, err
	}
	if len(data) > maximumStateBytes {
		return sharedBackendState{}, fmt.Errorf("shared backend ownership state exceeds size limit")
	}
	return decodeSharedBackendState(data)
}
