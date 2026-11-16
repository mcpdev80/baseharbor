package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const detachedTargetSelectionPrefix = "!detached:"

func targetSelectionDetached(raw []byte) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(text, detachedTargetSelectionPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(text, detachedTargetSelectionPrefix)
	return name, deployment.ValidateTargetName(name) == nil
}

func targetSelectionDetachedError(name string) error {
	return &machine.Error{Code: machine.ErrorConflict, CauseCode: "target_selection_detached", Message: "The selected Target definition was detached; automatic access remains disabled.", Resource: name, Next: "Restore the preserved Target definition, or explicitly activate another Target or deactivate selection. No alternate runtime or authentication is selected automatically."}
}

func lockTargetSelectionOwner() (func(), error) {
	path, err := targetSelectionPath()
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	if _, err := runtimeMutationOwnerBytes(path, true); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || !bhruntime.ProtectedStateOwner(root, info) {
		return nil, runtimeMutationOwnerError()
	}
	if _, err := runtimeMutationOwnerBytes(filepath.Join(root, ".installation.lock"), true); err != nil {
		return nil, err
	}
	return coreinstallation.AcquireStateFileLock(root)
}

// Called under the existing configuration lock. A stale selection cannot
// authorize substituting the current user's runtime/access choice.
func writeTargetSelectionExpected(path string, expected, desired []byte) error {
	current, err := runtimeMutationOwnerBytes(path, true)
	if err != nil {
		return err
	}
	if (current == nil) != (expected == nil) || !bytes.Equal(current, expected) {
		return deployment.ErrConfigurationChanged
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".active-target-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(desired); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	current, err = runtimeMutationOwnerBytes(path, true)
	if err != nil {
		return err
	}
	if (current == nil) != (expected == nil) || !bytes.Equal(current, expected) {
		return deployment.ErrConfigurationChanged
	}
	return os.Rename(file.Name(), path)
}

func mutatePersistedTarget(name string, clear bool) error {
	unlock, err := lockTargetSelectionOwner()
	if err != nil {
		return err
	}
	defer unlock()
	path, err := targetSelectionPath()
	if err != nil {
		return err
	}
	before, err := runtimeMutationOwnerBytes(path, true)
	if err != nil {
		return err
	}
	if clear {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return writeTargetSelectionExpected(path, before, []byte(name+"\n"))
}
