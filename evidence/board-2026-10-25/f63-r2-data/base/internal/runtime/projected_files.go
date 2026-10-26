package runtime

import (
	"os"
	"path/filepath"
)

func normalizeProjectedFiles(dirs ...string) error {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			if err := os.Chmod(filepath.Join(dir, entry.Name()), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
