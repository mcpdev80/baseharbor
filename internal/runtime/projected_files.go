package runtime

import (
	"os"
	"path/filepath"
)

// WriteContainerReadableProjection writes a non-secret configuration or a
// consumer-scoped projection and explicitly sets its final readable mode.
// Callers must keep sensitive projections below a protected host parent and
// mount each projection read-only into its authorized consumer.
func WriteContainerReadableProjection(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}

// NormalizeContainerReadableProjection makes a provider-owned bind-mount
// directory and its regular files readable by the declared container
// consumer, independent of the host process umask. Callers must keep the
// directory below a protected host-side owner directory and mount it only into
// its authorized consumer; private source material must never be placed in a
// shared projection.
func NormalizeContainerReadableProjection(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		return err
	}
	return normalizeProjectedFiles(dir)
}

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
