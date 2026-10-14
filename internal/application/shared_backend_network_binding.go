package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// A standalone SQL/PKI binding stays independent of the management installation.
// Only its transport joins an already materialized, protected Core network.
// The persisted binding keeps that network out of shared-backend retirement.
func bindSharedBackendNetwork(ctx context.Context, provider bhruntime.RuntimeProvider, dataDir string, files SharedBackendFiles, state *sharedBackendState) error {
	if state.CoreSQL != nil {
		return nil // The existing Core SQL dependency already owns this binding.
	}
	root := filepath.Join(dataDir, "runtime")
	core, err := bhruntime.ExistingControlPlaneRuntimeInventory(root, files.Project)
	if errors.Is(err, os.ErrNotExist) && !state.NetworkExternal {
		if _, rootErr := os.Lstat(root); errors.Is(rootErr, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("verify shared backend Core network owner: %w", err)
	}
	if core.Files.Project != files.Project || core.Files.ResourceProject != files.ResourceProject {
		return errors.New("shared backend network differs from its protected Core Target")
	}
	network := bhruntime.ProjectResource{Kind: "network", Name: files.Network}
	if !slices.Contains(core.Resources, network) {
		if state.NetworkExternal {
			return errors.New("shared backend external network has no protected Core owner")
		}
		return nil // Core itself joins the existing standalone shared network.
	}
	actual, err := provider.InspectProjectResources(ctx, files.Project, []bhruntime.ProjectResource{network})
	if err != nil {
		return fmt.Errorf("verify native shared backend Core network authority: %w", err)
	}
	if len(actual) != 1 || actual[0] != network {
		return errors.New("Core-owned shared network is not materialized; converge its owning installation before retrying")
	}
	state.NetworkExternal = true
	return nil
}
