package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

type RuntimeServiceStopProvider interface {
	InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error)
	ListRuntimeContainers(context.Context) ([]RuntimeContainer, error)
	StopProjectFilesSelected(context.Context, string, string, map[string]string, []string, ...string) error
	Kind() ProviderKind
}

type RuntimeServiceStopResult struct {
	Stopped  []ProjectResource `json:"stopped"`
	Retained []ProjectResource `json:"retained"`
	Verified bool              `json:"verified"`
}

// Stop only the caller's canonical module services. Consolidated projects may
// also contain other providers: neither project down nor project-wide stop is
// authority over those siblings. Persistent resources and process identities
// are inspected again even when the native command reports failure.
func StopOwnedProjectServices(ctx context.Context, provider RuntimeServiceStopProvider, project, workdir string, environment map[string]string, expected []ProjectResource, services []string, compose string) (RuntimeServiceStopResult, error) {
	r := RuntimeServiceStopResult{Stopped: []ProjectResource{}, Retained: []ProjectResource{}}
	if provider == nil || project == "" || workdir == "" || compose == "" || len(expected) == 0 || len(services) == 0 {
		return r, errors.New("selected stop requires canonical module ownership")
	}
	planned := map[ProjectResource]bool{}
	for _, resource := range expected {
		if resource.Name == "" || planned[resource] || (resource.Kind != "container" && resource.Kind != "volume" && resource.Kind != "network") {
			return r, errors.New("selected stop inventory is invalid")
		}
		planned[resource] = true
	}
	selected := map[string]bool{}
	for _, service := range services {
		if service == "" || selected[service] || !planned[ProjectResource{Kind: "container", Name: project + "-" + service + "-1"}] {
			return r, errors.New("selected stop service differs from canonical resources")
		}
		selected[service] = true
	}
	before, err := provider.InspectProjectResources(ctx, project, expected)
	if err != nil {
		return r, err
	}
	for _, resource := range before {
		if !planned[resource] {
			return r, errors.New("selected stop observed an unplanned resource")
		}
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return r, err
	}
	baseline := map[string]RuntimeContainer{}
	live := map[string]bool{}
	for _, container := range containers {
		if container.Project != project {
			continue
		}
		if _, duplicate := baseline[container.Name]; duplicate {
			return r, errors.New("selected stop has ambiguous native containers")
		}
		baseline[container.Name] = container
		if !selected[container.Service] {
			continue
		}
		resource := ProjectResource{Kind: "container", Name: project + "-" + container.Service + "-1"}
		if !ContainerMatchesProjectResource(provider.Kind(), container, project, resource) {
			return r, errors.New("selected stop has an unplanned service replica")
		}
		live[container.Service] = true
	}
	// Missing containers remain missing. Do not materialize them just to stop.
	actualServices := []string{}
	for service := range live {
		actualServices = append(actualServices, service)
	}
	sort.Strings(actualServices)
	var mutationErr error
	if len(actualServices) > 0 {
		mutationErr = provider.StopProjectFilesSelected(ctx, project, workdir, environment, actualServices, compose)
	}
	after, inspectErr := provider.InspectProjectResources(ctx, project, expected)
	remaining := map[ProjectResource]bool{}
	for _, resource := range after {
		if !planned[resource] {
			inspectErr = errors.Join(inspectErr, errors.New("stop verification observed unplanned resources"))
		}
		remaining[resource] = true
	}
	for _, resource := range before {
		if !remaining[resource] {
			inspectErr = errors.Join(inspectErr, fmt.Errorf("selected stop lost retained %s %s", resource.Kind, resource.Name))
		}
	}
	actual, listErr := provider.ListRuntimeContainers(ctx)
	observed := map[string]RuntimeContainer{}
	for _, container := range actual {
		if container.Project == project {
			observed[container.Name] = container
		}
	}
	for name, prior := range baseline {
		current, exists := observed[name]
		if !exists || current.ID != prior.ID || current.Project != prior.Project || current.Service != prior.Service {
			listErr = errors.Join(listErr, errors.New("selected stop changed retained native process ownership"))
			continue
		}
		if selected[prior.Service] {
			if current.Running || (current.State != "exited" && current.State != "stopped" && current.State != "created") {
				listErr = errors.Join(listErr, errors.New("selected service is not verified stopped"))
			} else {
				r.Stopped = append(r.Stopped, ProjectResource{Kind: "container", Name: project + "-" + prior.Service + "-1"})
			}
		} else if current.Running != prior.Running || current.State != prior.State {
			listErr = errors.Join(listErr, errors.New("selected stop changed a sibling service"))
		}
	}
	r.Retained = after
	err = errors.Join(mutationErr, inspectErr, listErr)
	r.Verified = err == nil
	return r, err
}
