package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// The independently selected management installation can join a network that
// the existing shared backend owner created first. Labels alone are not an
// authority: require its exact protected renderer and actual native ownership.
func inspectCoreDefaultNetworkBinding(ctx context.Context, target deployment.ResolvedTarget, files bhruntime.Files) (bool, error) {
	dataRoot, err := targetDataRoot(target)
	if err != nil {
		return false, err
	}
	shared, sharedErr := application.ExistingSharedBackendInventoryAt(dataRoot, target.Name, "core")
	if sharedErr != nil && !errors.Is(sharedErr, os.ErrNotExist) {
		return false, fmt.Errorf("verify existing shared network owner: %w", sharedErr)
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return false, err
	}
	network := bhruntime.ProjectResource{Kind: "network", Name: bhruntime.ControlPlaneNetworkName(files.ResourceProject)}
	actual, err := provider.InspectProjectResources(ctx, files.Project, []bhruntime.ProjectResource{network})
	if err != nil {
		return false, fmt.Errorf("verify native Core network authority: %w", err)
	}
	if sharedErr == nil && shared.CoreSQL == nil {
		declared := false
		for _, resource := range shared.Resources {
			declared = declared || resource == network
		}
		if shared.Files.Project != files.Project || shared.Files.Network != network.Name || !declared {
			return false, errors.New("existing shared owner does not authorize the selected Core network")
		}
		if len(actual) != 1 || actual[0] != network {
			return false, errors.New("existing shared backend network is not materialized; converge its owning application before retrying Core setup")
		}
		return true, nil
	}
	if files.DefaultNetworkExternal {
		return false, errors.New("protected Core shared-network binding has no verifiable shared backend owner")
	}
	if len(actual) != 0 {
		// Existing Core-owned default networks remain unchanged. A matching
		// native label without that protected owner must never be adopted.
		if _, err := bhruntime.ExistingControlPlaneRuntimeInventory(filepath.Join(dataRoot, "runtime"), files.Project); err != nil {
			return false, errors.New("existing native network lacks a protected Core or shared backend owner")
		}
	}
	return false, nil
}

func prepareCoreDefaultNetworkBinding(ctx context.Context, target deployment.ResolvedTarget, files bhruntime.Files) (bhruntime.Files, error) {
	external, err := inspectCoreDefaultNetworkBinding(ctx, target, files)
	if err != nil || !external {
		return files, err
	}
	return bhruntime.BindControlPlaneToExistingDefaultNetwork(files)
}
