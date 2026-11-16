package runtime

import (
	"errors"
	"os"
	"path/filepath"
)

func ControlPlaneServiceAccessNeedsBootstrapRestart(files Files) (bool, error) {
	stateDir := filepath.Dir(files.Compose)
	for _, path := range []string{
		filepath.Join(stateDir, "providers", "postgresql", "service-access", "pki", "state.json"),
		filepath.Join(stateDir, "providers", "openbao", "service-access", "pki", "state.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			continue
		} else if errors.Is(err, os.ErrNotExist) {
			return true, nil
		} else {
			return false, err
		}
	}
	return false, nil
}
