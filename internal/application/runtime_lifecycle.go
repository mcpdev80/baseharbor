package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

var ErrRuntimeDefinitionChanged = errors.New("application runtime definition differs from the BaseHarbor-managed definition")

func ExpectedRuntimeResources(m Manifest) []bhruntime.ProjectResource {
	return ExpectedRuntimeResourcesForProject(m, RuntimeProjectName(m))
}

func ExpectedRuntimeResourcesForProject(m Manifest, project string) []bhruntime.ProjectResource {
	return ExpectedRuntimeResourcesForIdentity(m, project, project)
}

func ExpectedRuntimeResourcesForIdentity(m Manifest, composeProject, resourceProject string) []bhruntime.ProjectResource {
	if !HasApplicationScopedRuntimeServices(m) {
		return nil
	}
	resources := []bhruntime.ProjectResource{{Kind: "network", Name: ApplicationBackendNetworkNameForProject(resourceProject)}}
	if !UsesSharedPostgreSQL(m) {
		for _, instance := range SQLInstanceNames(m) {
			service := runtimeServiceName("postgres", instance)
			resources = append(resources,
				bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-" + service + "-1"},
				bhruntime.ProjectResource{Kind: "volume", Name: resourceProject + "_" + service + "-data"},
			)
		}
	}
	if !UsesSharedValkey(m) {
		for _, instance := range ValkeyInstanceNames(m) {
			service := runtimeServiceName("valkey", instance)
			resources = append(resources,
				bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-" + service + "-1"},
				bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-" + valkeyAccessService(instance) + "-1"},
				bhruntime.ProjectResource{Kind: "volume", Name: resourceProject + "_" + service + "-data"},
			)
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		service := runtimeServiceName("rabbitmq", instance)
		resources = append(resources,
			bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-" + service + "-1"},
			bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-" + rabbitmqAccessService(instance) + "-1"},
			bhruntime.ProjectResource{Kind: "volume", Name: resourceProject + "_" + service + "-data"},
		)
	}
	if m.Services.SQLManagementUI && !UsesSharedPostgreSQL(m) {
		resources = append(resources, bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-postgres-ui-1"})
	}
	if m.Services.CacheManagementUI && !UsesSharedValkey(m) {
		resources = append(resources,
			bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-cache-ui-1"},
			bhruntime.ProjectResource{Kind: "container", Name: composeProject + "-cache-ui-access-1"},
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
	return ExpectedPersistentRuntimeResourcesForProject(m, RuntimeProjectName(m))
}

func ExpectedPersistentRuntimeResourcesForProject(m Manifest, project string) []bhruntime.ProjectResource {
	var resources []bhruntime.ProjectResource
	for _, resource := range ExpectedRuntimeResourcesForProject(m, project) {
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
	resourceProject := strings.TrimSpace(files.ResourceProject)
	if resourceProject == "" {
		resourceProject = RuntimeProjectName(m)
	}
	expected, err := RuntimeComposeYAMLForProject(m, resourceProject)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, []byte(expected)) {
		return ErrRuntimeDefinitionChanged
	}
	return nil
}

func InspectOwnedRuntimeResources(ctx context.Context, compose bhruntime.RuntimeProvider, m Manifest) ([]bhruntime.ProjectResource, error) {
	return compose.InspectProjectResources(ctx, RuntimeProjectName(m), ExpectedRuntimeResources(m))
}

func InspectOwnedRuntimeResourcesForFiles(ctx context.Context, compose bhruntime.RuntimeProvider, m Manifest, files RuntimeFiles) ([]bhruntime.ProjectResource, error) {
	resourceProject := strings.TrimSpace(files.ResourceProject)
	if resourceProject == "" {
		resourceProject = RuntimeProjectName(m)
	}
	return compose.InspectProjectResources(ctx, files.Project, ExpectedRuntimeResourcesForIdentity(m, files.Project, resourceProject))
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
