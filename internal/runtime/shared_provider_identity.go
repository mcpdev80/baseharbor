package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CheckSharedProviderIdentity prevents new logical sharing boundaries from
// replacing retained, separately deployed physical providers. It is read-only.
func CheckSharedProviderIdentity(dataDir, provider string) error {
	root := filepath.Join(filepath.Clean(dataDir), "providers", provider, "shared")
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("shared provider state must be a canonical directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("shared provider state contains a noncanonical binding")
		}
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, entry.Name(), "compose.yaml")); err == nil {
			return fmt.Errorf("retained shared %s has a separately deployed sharing boundary; explicit backup-verified migration is required and currently unsupported; existing files and volumes are retained", provider)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
