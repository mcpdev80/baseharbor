package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrRuntimeNotApplied = errors.New("application runtime has not been applied")

func ExistingRuntimeFiles(store Store, m Manifest) (RuntimeFiles, error) {
	if err := m.Validate(); err != nil {
		return RuntimeFiles{}, err
	}
	dir := filepath.Join(store.Root, m.Name, "runtime")
	files := RuntimeFiles{
		Dir:            dir,
		Compose:        filepath.Join(dir, "compose.yaml"),
		Env:            filepath.Join(dir, "runtime.env"),
		ApplicationEnv: filepath.Join(dir, "application.env"),
		Bindings:       filepath.Join(dir, "bindings"),
	}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return RuntimeFiles{}, fmt.Errorf("%w: run 'baha app apply %s' first", ErrRuntimeNotApplied, m.Name)
			}
			return RuntimeFiles{}, fmt.Errorf("inspect application runtime state: %w", err)
		}
	}
	return files, nil
}

func CheckRuntimePermissions(files RuntimeFiles) error {
	paths := []string{files.Dir, files.Compose, files.Env}
	for _, optional := range []string{files.ApplicationEnv, files.Bindings, filepath.Join(files.Dir, "openbao.env")} {
		if optional == "" {
			continue
		}
		if _, err := os.Stat(optional); err == nil {
			paths = append(paths, optional)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("%s is accessible by group or others (%o)", path, info.Mode().Perm())
		}
	}
	return nil
}
