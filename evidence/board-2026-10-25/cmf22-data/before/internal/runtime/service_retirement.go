package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RuntimeServiceRetirementProvider retires a selected service set without
// bringing down sibling modules consolidated under the same Compose project.
type RuntimeServiceRetirementProvider interface {
	InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error)
	ListRuntimeContainers(context.Context) ([]RuntimeContainer, error)
	RetireOwnedProjectContainers(context.Context, string, []ProjectResource) error
}

// RetireOwnedServices accepts the canonical old resource set and exactly the
// containers released by the consumer plan. Volumes and networks are never
// deletion targets. Failed mutations still return observed partial disposition.
func RetireOwnedServices(ctx context.Context, provider RuntimeServiceRetirementProvider, project string, expected, remove []ProjectResource) (RuntimeRetirement, error) {
	result := RuntimeRetirement{Removed: []ProjectResource{}, Retained: []RetainedRuntimeResource{}}
	if provider == nil || strings.TrimSpace(project) == "" || len(expected) == 0 {
		return result, errors.New("service retirement requires canonical owner inventory")
	}
	planned := map[ProjectResource]bool{}
	for _, resource := range expected {
		if strings.TrimSpace(resource.Name) == "" || planned[resource] || (resource.Kind != "container" && resource.Kind != "network" && resource.Kind != "volume") {
			return result, errors.New("service retirement inventory is invalid or duplicated")
		}
		planned[resource] = true
	}
	services := map[string]bool{}
	removed := map[ProjectResource]bool{}
	for _, resource := range remove {
		if resource.Kind != "container" || !planned[resource] || removed[resource] || !strings.HasPrefix(resource.Name, project+"-") || !strings.HasSuffix(resource.Name, "-1") {
			return result, errors.New("service retirement requires exact canonical container names")
		}
		service := strings.TrimSuffix(strings.TrimPrefix(resource.Name, project+"-"), "-1")
		if service == "" {
			return result, errors.New("service retirement requires a canonical service")
		}
		services[service], removed[resource] = true, true
	}
	before, err := provider.InspectProjectResources(ctx, project, expected)
	if err != nil {
		return result, fmt.Errorf("verify selected service ownership: %w", err)
	}
	for _, resource := range before {
		if !planned[resource] {
			return result, errors.New("service retirement observation contains an unplanned resource")
		}
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return result, err
	}
	kind := ProviderDocker
	if native, ok := provider.(interface{ Kind() ProviderKind }); ok {
		kind = native.Kind()
	}
	for _, container := range containers {
		if container.Project == project && services[container.Service] && !ContainerMatchesProjectResource(kind, container, project, ProjectResource{Kind: "container", Name: project + "-" + container.Service + "-1"}) {
			return result, fmt.Errorf("unplanned replica of selected service %s; preserve ownership state", container.Service)
		}
	}
	var mutationErr error
	if len(remove) > 0 {
		// Pass only the explicitly selected containers. The existing native
		// adapter rechecks their ownership and never receives a data volume.
		mutationErr = provider.RetireOwnedProjectContainers(ctx, project, remove)
	}
	after, err := provider.InspectProjectResources(ctx, project, expected)
	if err != nil {
		return result, errors.Join(mutationErr, fmt.Errorf("verify selected retirement; preserve ownership for retry: %w", err))
	}
	remaining := map[ProjectResource]bool{}
	var verificationErr error
	for _, resource := range after {
		if !planned[resource] {
			return result, errors.Join(mutationErr, errors.New("service retirement verification contains an unplanned resource"))
		}
		remaining[resource] = true
		if removed[resource] {
			verificationErr = errors.Join(verificationErr, fmt.Errorf("retired container remains: %s", resource.Name))
		} else {
			reason := "resource remains with its existing owner for active consumers and retry"
			if resource.Kind == "network" {
				reason = "owned network remains as existing module/retry state; selected container retirement does not verify attachment release or authorize network reclamation"
			}
			if resource.Kind == "volume" {
				reason = "persistent volume remains at its existing location; data deletion requires separate explicit consent and ownership proof"
			}
			result.Retained = append(result.Retained, RetainedRuntimeResource{Resource: resource, Reason: reason})
		}
	}
	for _, resource := range before {
		if remaining[resource] {
			continue
		}
		if !removed[resource] {
			verificationErr = errors.Join(verificationErr, fmt.Errorf("selected retirement lost non-selected %s %s; preserve recovery state", resource.Kind, resource.Name))
		}
		result.Removed = append(result.Removed, resource)
	}
	return result, errors.Join(mutationErr, verificationErr)
}
