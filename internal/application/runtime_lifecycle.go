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

func ExpectedRuntimeResources(m Manifest) []bhruntime.ProjectResource {
	if !HasManagedRuntimeServices(m) {
		return nil
	}
	project := RuntimeProjectName(m)
	resources := []bhruntime.ProjectResource{{Kind: "network", Name: ApplicationBackendNetworkName(m)}}
	for _, instance := range PostgresInstanceNames(m) {
		service := runtimeServiceName("postgres", instance)
		resources = append(resources,
			bhruntime.ProjectResource{Kind: "container", Name: project + "-" + service + "-1"},
			bhruntime.ProjectResource{Kind: "volume", Name: project + "_" + service + "-data"},
		)
	}
	for _, instance := range RedisInstanceNames(m) {
		service := runtimeServiceName("valkey", instance)
		resources = append(resources,
			bhruntime.ProjectResource{Kind: "container", Name: project + "-" + service + "-1"},
			bhruntime.ProjectResource{Kind: "volume", Name: project + "_" + service + "-data"},
		)
	}
	return resources
}

// ExpectedPostgresRuntimeResources is kept for compatibility with the first
// runtime milestone. New lifecycle code should use ExpectedRuntimeResources.
func ExpectedPostgresRuntimeResources(m Manifest) []bhruntime.ProjectResource {
	return ExpectedRuntimeResources(m)
}

func ExpectedPersistentRuntimeResources(m Manifest) []bhruntime.ProjectResource {
	var resources []bhruntime.ProjectResource
	for _, resource := range ExpectedRuntimeResources(m) {
		if resource.Kind == "volume" {
			resources = append(resources, resource)
		}
	}
	return resources
}

// CheckManagedRuntimeDefinition prevents destructive operations from trusting a
// locally modified Compose file. BaseHarbor may only mutate the runtime shape it
// originally generated and understands for the current manifest.
func CheckManagedRuntimeDefinition(files RuntimeFiles, m Manifest) error {
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		return fmt.Errorf("read application compose definition: %w", err)
	}
	expected, err := RuntimeComposeYAML(m)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, []byte(expected)) {
		return ErrRuntimeDefinitionChanged
	}
	return nil
}

func InspectOwnedRuntimeResources(ctx context.Context, compose bhruntime.Compose, m Manifest) ([]bhruntime.ProjectResource, error) {
	project := RuntimeProjectName(m)
	var existing []bhruntime.ProjectResource
	for _, resource := range ExpectedRuntimeResources(m) {
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

func ResourceNamedExists(resources []bhruntime.ProjectResource, wanted bhruntime.ProjectResource) bool {
	for _, resource := range resources {
		if resource.Kind == wanted.Kind && resource.Name == wanted.Name {
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
