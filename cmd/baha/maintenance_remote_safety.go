package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

const minMaintenanceSecretBytes = 12

// maintenanceSecretTransport is the only channel this client accepts for a
// client-local secret reference. Production reads the file directly; tests
// inject a capture so the assertion cannot be satisfied by a path claim.
var maintenanceSecretTransport = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// readMaintenanceSecretInput transfers one client-local secret reference over the
// explicit safe channel and proves the reference is usable. The returned bytes
// are used only as a key input, never printed, logged or placed in a machine
// result, and the caller must zero them.
func readMaintenanceSecretInput(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, applicationMaintenancePasswordFileRequired()
	}
	if !filepath.IsAbs(path) {
		return nil, machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Maintenance secret input %q is a client filename, not a safe channel reference.", redactMaintenanceInput(path)),
			"Reference an owner-only local password file; a plaintext value is never accepted as secret input.", false)
	}
	if filepath.Clean(path) != path {
		return nil, machine.NewError(machine.ErrorValidationFailed,
			"Maintenance secret input is not a clean client-local path.",
			"Pass the canonical absolute path of the owner-only password file.", false)
	}
	data, err := maintenanceSecretTransport(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, machine.NewError(machine.ErrorValidationFailed,
				"The maintenance password file does not exist on the client.",
				"Create an owner-only local password file; the authority never reads one it was only told about.", false)
		}
		return nil, fmt.Errorf("read maintenance secret input: %w", err)
	}
	defer zeroBytes(data)
	secret := trimMaintenanceSecretBytes(data)
	if len(secret) < minMaintenanceSecretBytes {
		zeroBytes(secret)
		return nil, applicationMaintenanceSecretInputTooShort()
	}
	return secret, nil
}

func trimMaintenanceSecretBytes(data []byte) []byte {
	return []byte(strings.TrimRight(string(data), "\r\n"))
}

func applicationMaintenanceSecretInputTooShort() error {
	return machine.NewError(machine.ErrorValidationFailed,
		"Maintenance secret input is shorter than the supported recovery-key minimum.",
		"Provide an owner-only password file with at least twelve bytes.", false)
}

// maintenanceFileDigest binds a window to the exact bytes the client holds, so a
// replaced archive or password file is never accepted as the same operation. The
// digest of a secret file is never the secret itself.
func maintenanceFileDigest(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return "sha256:invalid"
	}
	file, err := os.Open(path)
	if err != nil {
		return "sha256:unreadable"
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "sha256:unreadable"
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "sha256:unreadable"
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil))
}
