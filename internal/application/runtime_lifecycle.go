package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrRuntimeDefinitionChanged = errors.New("application runtime definition differs from the BaseHarbor-managed definition")

func ExpectedPostgresRuntimeResources(m Manifest) []bhruntime.ProjectResource {
	project := RuntimeProjectName(m)
	return []bhruntime.ProjectResource{
		{Kind: "container", Name: project + "-postgres-1"},
		{Kind: "network", Name: project + "_default"},
		{Kind: "volume", Name: project + "_postgres-data"},
	}
}

// CheckManagedRuntimeDefinition prevents destructive operations from trusting a
// locally modified Compose file. BaseHarbor may only destroy the runtime shape
// it originally generated and understands.
func CheckManagedRuntimeDefinition(files RuntimeFiles) error {
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		return fmt.Errorf("read application compose definition: %w", err)
	}
	if !bytes.Equal(data, []byte(postgresComposeYAML)) {
		return ErrRuntimeDefinitionChanged
	}
	return nil
}

func InspectOwnedRuntimeResources(ctx context.Context, compose bhruntime.Compose, m Manifest) ([]bhruntime.ProjectResource, error) {
	project := RuntimeProjectName(m)
	var existing []bhruntime.ProjectResource
	for _, resource := range ExpectedPostgresRuntimeResources(m) {
		exists, err := compose.InspectProjectResource(ctx, project, resource)
		if err != nil {
			return nil, err
		}
		if exists {
			existing = append(existing, resource)
		}
	}
	return existing, nil
}

func ResourceExists(resources []bhruntime.ProjectResource, kind string) bool {
	for _, resource := range resources {
		if resource.Kind == kind {
			return true
		}
	}
	return false
}

func (s Store) Delete(name string) error {
	if err := validateSlug("application name", name); err != nil {
		return err
	}
	appDir := filepath.Join(s.Root, name)
	if _, err := os.Stat(appDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("application %q not found", name)
		}
		return err
	}
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("remove application state: %w", err)
	}
	return nil
}
