package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func (e *applicationDestroyExecution) destroyRepositoryDataVolumes(ctx context.Context) error {
	project := application.WorkloadProjectNameForRuntime(e.manifest, e.files)
	resources := exclusivelyOwnedRepositoryVolumes(e.repositoryVolumes, project)
	if len(resources) == 0 {
		return nil
	}
	if err := e.compose.DestroyOwnedProjectResources(ctx, project, resources); err != nil {
		return fmt.Errorf("remove exclusively application-owned Compose data volumes: %w", err)
	}
	remaining, err := e.compose.InspectProjectResources(ctx, project, resources)
	if err != nil {
		return fmt.Errorf("verify application data volume removal: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("verify application data volume removal: %d exclusively owned volume(s) remain", len(remaining))
	}
	return nil
}

func exclusivelyOwnedRepositoryVolumes(volumes []bhruntime.RepositoryVolume, project string) []bhruntime.ProjectResource {
	resources := make([]bhruntime.ProjectResource, 0, len(volumes))
	for _, volume := range volumes {
		if volume.Name != "" && volume.Project == project && !volume.Shared {
			resources = append(resources, bhruntime.ProjectResource{Kind: "volume", Name: volume.Name})
		}
	}
	return resources
}

func renderRepositoryDestroyVolumes(out io.Writer, volumes []bhruntime.RepositoryVolume, project string) {
	for _, volume := range volumes {
		if volume.Project == project && !volume.Shared {
			fmt.Fprintf(out, "  delete data volume: %s (verified application ownership)\n", volume.Name)
			continue
		}
		fmt.Fprintf(out, "  preserve volume: %s (%s; ownership is shared or unverified)\n", volume.Name, volume.Reason)
	}
}
