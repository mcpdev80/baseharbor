package main

import (
	"errors"
	"os"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// A provider-only application can have a verified deployment without any local
// runtime projection. Do not create one just to retire it. Missing nested files
// and other inventory failures still fail; existing material is inspected normally.
func (e *applicationDestroyExecution) observeRetirementRuntimeMaterials() ([]application.ProviderMaterial, error) {
	if _, err := os.Lstat(e.files.Dir); errors.Is(err, os.ErrNotExist) && errors.Is(e.runtimeErr, application.ErrRuntimeNotApplied) {
		// Reuse the protected-owner reader's parent checks so a dangling
		// or redirected ancestor cannot masquerade as an absent projection.
		if _, err := runtimeMutationOwnerBytes(e.files.Dir, true); err != nil {
			return nil, err
		}
		return []application.ProviderMaterial{}, nil
	} else if err != nil {
		return nil, err
	}
	retained, _, err := observeOwnedProviderFiles(e.manifest.ApplicationID, e.resolved.Target.Name, "deployment/"+e.resolved.DeploymentIdentity.DeploymentID, e.files.Dir)
	return retained, err
}
