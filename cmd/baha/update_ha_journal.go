package main

import (
	"errors"
	"os"
	"path/filepath"
)

// A recovery's DCS directories become active bind mounts. Never rename or
// overwrite them when retrying an update; begin a new nested transaction.
func coreUpdateJournal(root, release string, fresh bool) (string, error) {
	path := filepath.Join(root, "core-updates", safeVersionPathPart(release))
	for depth := 0; depth < 16; depth++ {
		if _, err := privateRecoveryFile(filepath.Join(path, "ha-cutover-committed")); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
		next := filepath.Join(path, "update-after-recovery")
		if fresh {
			if err := os.MkdirAll(next, 0700); err != nil {
				return "", err
			}
		} else if _, err := privateRecoveryFile(filepath.Join(next, "ha-recovery-source.json")); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
		st, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
			return "", errors.New("unsafe subsequent update journal")
		}
		path = next
	}
	return "", errors.New("too many recovered update attempts; reconciliation required")
}
