package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// maintenanceTombstonePath resolves the durable maintenance tombstone for a
// deployment: a sibling of the deployment record that DeleteDeploymentRecord
// does NOT remove. The deployment record lives at
// <DataRoot>/targets/<target>/deployments/<deploymentID>; the tombstone lives at
// <DataRoot>/targets/<target>/maintenance/<deploymentID>.json, one level up from
// the deployments subtree, so os.RemoveAll of the deployment root cannot delete
// the tombstone and the destroy outcome stays queryable after teardown.
func maintenanceTombstonePath(resolved resolvedApplication) (string, error) {
	root, err := deployment.DataRoot()
	if err != nil {
		return "", err
	}
	return maintenanceTombstonePathFor(root, resolved.Target.Name, resolved.DeploymentIdentity.DeploymentID), nil
}

// maintenanceTombstonePathFor is the same layout resolved from a data root and
// an explicit identity: the production replay reader and the writer must agree
// on exactly one path, so neither derives the layout a second time.
func maintenanceTombstonePathFor(dataRoot, target, deploymentID string) string {
	tombstoneDir := filepath.Join(dataRoot, "targets", target, "maintenance")
	return filepath.Join(tombstoneDir, deploymentID+".json")
}

func init() {
	// Wire the production replay reader: the destroy route stamps a durable
	// tombstone that DeleteDeploymentRecord does not remove, and the revision
	// lookup below the (now gone) deployment root reads it back. Registration
	// in init means every production route — commit-time replay included —
	// answers a destroyed deployment from its tombstone, not only a helper a
	// test calls.
	application.SetMaintenanceTombstoneReplay(maintenanceTombstonePathForTarget, maintenanceStateRootIsOwned)
}

// maintenanceTombstonePathForTarget is the registered resolver: it addresses
// the tombstone namespace for one (target, deploymentID) identity under the
// installation data root.
func maintenanceTombstonePathForTarget(target, deploymentID string) string {
	root, err := deployment.DataRoot()
	if err != nil {
		return ""
	}
	return maintenanceTombstonePathFor(root, target, deploymentID)
}

// maintenanceStateRootIsOwned reports whether a state root is the protected
// state directory of a deployment this installation owns: it must live under
// the installation data root, and either its deployment record (live or
// incomplete root) or its durable maintenance tombstone must exist. An
// arbitrary caller path is therefore never answered from a tombstone.
//
// It must not read a deployment-record *file*: DeleteDeploymentRecord removes
// the whole deployment root, so the record file is exactly what a destroyed
// deployment no longer has. Ownership lives in the installation record of the
// Target (the one-installation-owner rule), which survives teardown.
func maintenanceStateRootIsOwned(stateRoot, target, deploymentID string) bool {
	dataRoot, err := deployment.DataRoot()
	if err != nil || !filepath.IsAbs(stateRoot) || filepath.Clean(stateRoot) != stateRoot {
		return false
	}
	prefix := filepath.Join(dataRoot, "targets") + string(filepath.Separator)
	rest, ok := strings.CutPrefix(stateRoot, prefix)
	if !ok || rest == "" || strings.Contains(rest, "..") {
		return false
	}
	if deployment.ValidateTargetName(target) != nil || deployment.ValidateDeploymentID(deploymentID) != nil {
		return false
	}
	if err := deployment.VerifyDeploymentOwner(target, filepath.Join(dataRoot, "targets", target, "runtime"), deploymentID); err != nil {
		return false
	}
	return true
}

// stampMaintenanceTombstone writes a durable tombstone copy of the committed
// destroy window so the completed operation stays queryable after the destroy
// owner removes the deployment root. The current window record (already written
// inside the deployment root) is mirrored here at a sibling path that
// DeleteDeploymentRecord does not touch. A write failure is surfaced loudly: a
// destroy must never claim a completed outcome with no durable trace of it.
func stampMaintenanceTombstone(resolved resolvedApplication, record application.MaintenanceAuthorityRecord) error {
	path, err := maintenanceTombstonePath(resolved)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create maintenance tombstone directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode maintenance tombstone: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write maintenance tombstone: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("protect maintenance tombstone: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit maintenance tombstone: %w", err)
	}
	return nil
}

// rollbackMaintenanceTombstone removes the tombstone a destroy wrote when the
// owner then failed to converge. A failed destroy must leave no tombstone
// claiming a completed outcome, so the rollback deletes the tombstone file if
// it exists.
func rollbackMaintenanceTombstone(resolved resolvedApplication) error {
	path, err := maintenanceTombstonePath(resolved)
	if err != nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rollback maintenance tombstone: %w", err)
	}
	return nil
}
