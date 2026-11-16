package openbao

import (
	"errors"
	"os"
)

// ValidateRecoveryFile performs a read-only rotation preflight without exposing
// unseal keys. HTTP selects this path from protected installation state.
func ValidateRecoveryFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64*1024 {
		return errors.New("protected regular recovery material is required")
	}
	if _, err := loadRecoveryFile(path); err != nil {
		return errors.New("valid recovery material is required")
	}
	return nil
}
