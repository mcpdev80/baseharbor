package devgateway

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

func projectReadable(source, target string) error {
	return projectReadableMode(source, target, 0o644)
}

func projectReadableMode(source, target string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%s is empty", filepath.Base(source))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func gatewayRuntimeIdentity() (string, string) {
	current, err := user.Current()
	if err == nil && numericIdentity(current.Uid) && numericIdentity(current.Gid) {
		return current.Uid, current.Gid
	}
	return "65532", "65532"
}

func numericIdentity(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
