package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

// authorizeUnownedLocalTarget grants the narrow owner path for local workload-
// only applications. Core ownership is resolved first by VerifyDeploymentOwner;
// this path is considered only for os.ErrNotExist and requires an explicit
// Scope=local + local-access binding, a Docker or Podman runtime, no tenant
// identity, and no pre-existing runtime contents that could be an orphaned
// Core installation.
func authorizeUnownedLocalTarget(targetName, targetRuntimeRoot string) (bool, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return false, err
	}
	target, err := cfg.ResolveTarget(targetName, "")
	if err != nil {
		return false, nil
	}
	runtimeProvider := strings.ToLower(strings.TrimSpace(target.RuntimeProvider))
	if target.Name != targetName || target.Scope != "local" || target.AccessProvider != string(targetaccess.ProviderLocal) || strings.TrimSpace(target.TenantID) != "" || (runtimeProvider != "docker" && runtimeProvider != "podman") {
		return false, nil
	}
	targetRoot, err := TargetStateRoot(targetName)
	if err != nil {
		return false, err
	}
	if filepath.Clean(targetRuntimeRoot) != filepath.Clean(filepath.Join(targetRoot, "runtime")) {
		return false, nil
	}
	// Check the target state root itself before inspecting runtime. Lstat on
	// targets/<name>/runtime follows a symlink at targets/<name>, which could
	// otherwise make an external empty directory look like a coreless target.
	stateInfo, err := os.Lstat(targetRoot)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !stateInfo.IsDir() || stateInfo.Mode()&os.ModeSymlink != 0 || stateInfo.Mode().Perm()&0077 != 0 {
		return false, nil
	}
	info, err := os.Lstat(targetRuntimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return false, nil
	}
	entries, err := os.ReadDir(targetRuntimeRoot)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// deploymentDirForTarget resolves the deployments namespace of a Target
// (targets/<target>/deployments) via the canonical layout.
func deploymentDirForTarget(targetName string) (string, error) {
	targetRoot, err := TargetStateRoot(targetName)
	if err != nil {
		return "", err
	}
	return filepath.Join(targetRoot, "deployments"), nil
}

// deploymentTargetFromDir extracts the Target segment (targets/<target>/...)
// from a deployment directory path, if the conventional layout is present.
func deploymentTargetFromDir(deploymentDir string) (string, bool) {
	segments := strings.Split(filepath.Clean(deploymentDir), string(filepath.Separator))
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] == "targets" && i+1 < len(segments) {
			return segments[i+1], true
		}
	}
	return "", false
}
