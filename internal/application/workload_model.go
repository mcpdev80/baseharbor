package application

import (
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/workload"
)

// ResolveRepositoryWorkloadModel translates the repository-owned workload
// source selected by the portable application contract into provider-neutral
// runtime semantics. It never mutates the source workload.
func ResolveRepositoryWorkloadModel(repositoryRoot string, m Manifest) (workload.Model, bool, error) {
	selected, composePath, found, err := SelectedWorkloadServices(repositoryRoot, m)
	if err != nil || !found {
		return workload.Model{}, found, err
	}
	model, err := workload.FromCompose(composePath, selected)
	if err != nil {
		return workload.Model{}, true, fmt.Errorf("normalize repository workload: %w", err)
	}
	return model, true, nil
}
