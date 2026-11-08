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
	Stopped              []ProjectResource `json:"stopped"`
	Retained             []ProjectResource `json:"retained"`
	Removed              []ProjectResource `json:"removed"`
	Verified             bool              `json:"verified"`
	VerificationFailures []string          `json:"verification_failures,omitempty"`
}

// Stop only the caller's canonical module services. Consolidated projects may
// also contain other providers: neither project down nor project-wide stop is
// authority over those siblings. Persistent resources and process identities
// are inspected again even when the native command reports failure.
func StopOwnedProjectServices(ctx context.Context, provider RuntimeServiceStopProvider, project, workdir string, environment map[string]string, expected []ProjectResource, services []string, composeFiles ...string) (RuntimeServiceStopResult, error) {
	r := RuntimeServiceStopResult{Stopped: []ProjectResource{}, Retained: []ProjectResource{}, Removed: []ProjectResource{}}
	if provider == nil || project == "" || workdir == "" || len(composeFiles) == 0 || len(expected) == 0 || len(services) == 0 {
		return r, errors.New("selected stop requires canonical module ownership")
	}
	for _, compose := range composeFiles {
		if compose == "" {
			return r, errors.New("selected stop requires canonical module ownership")
		}
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
	// Compute the validated planned native-name set once for each selected
	// service from the actual Compose render inputs, so both the initial
	// replica check below and the selected-stop postcondition share this exact
	// contract (0427-128: a pinned container_name lives outside
	// <project>-<service>-1 and must be accepted there too).
	plannedByService := map[string][]string{}
	if cfg, ok := provider.(RuntimeConfigProvider); ok && len(composeFiles) > 0 && provider.Kind() != ProviderPodman {
		if rendered, rerr := cfg.ConfigJSONProjectFilesEnv(ctx, project, workdir, nil, composeFiles...); rerr == nil && rendered != "" {
			for _, service := range services {
				if planned, perr := parsedExpectedContainerNames(rendered, project, service); perr == nil && len(planned) > 0 {
					plannedByService[service] = planned
				}
			}
		}
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
		plannedNames := plannedByService[container.Service]
		if provider.Kind() == ProviderPodman {
			plannedNames = append(plannedNames, project+"-"+container.Service, project+"-"+container.Service+"-1")
		}
		plannedNames = append(plannedNames, project+"-"+container.Service+"-1")
		if !containerNameInPlannedSet(provider.Kind(), container, project, plannedNames) {
			return r, fmt.Errorf("selected stop has an unplanned service replica (%s): %s", container.Service, container.Name)
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
		mutationErr = provider.StopProjectFilesSelected(ctx, project, workdir, environment, actualServices, composeFiles...)
	}
	if mutationErr != nil {
		r.VerificationFailures = append(r.VerificationFailures, "native-selected-stop-failed")
	}
	after, inspectErr := provider.InspectProjectResources(ctx, project, expected)
	if inspectErr != nil {
		r.VerificationFailures = append(r.VerificationFailures, "native-resource-inspection-failed")
	}
	remaining := map[ProjectResource]bool{}
	for _, resource := range after {
		if !planned[resource] {
			r.VerificationFailures = append(r.VerificationFailures, "unexpected-native-resource")
			inspectErr = errors.Join(inspectErr, errors.New("stop verification observed unplanned resources"))
		}
		remaining[resource] = true
	}
	for _, resource := range before {
		if !remaining[resource] {
			selectedContainer := false
			for service := range selected {
				selectedContainer = selectedContainer || resource == (ProjectResource{Kind: "container", Name: project + "-" + service + "-1"})
			}
			if selectedContainer {
				r.Removed = append(r.Removed, resource)
				continue
			}
			r.VerificationFailures = append(r.VerificationFailures, "retained-resource-missing:"+resource.Kind+"/"+resource.Name)
			inspectErr = errors.Join(inspectErr, fmt.Errorf("selected stop lost retained %s %s", resource.Kind, resource.Name))
		}
	}
	actual, listErr := provider.ListRuntimeContainers(ctx)
	if listErr != nil {
		r.VerificationFailures = append(r.VerificationFailures, "native-process-inspection-failed")
	}
	observed := map[string]RuntimeContainer{}
	for _, container := range actual {
		if container.Project == project {
			observed[container.Name] = container
			plannedNames := plannedByService[container.Service]
			plannedNames = append(plannedNames, project+"-"+container.Service+"-1")
			if selected[container.Service] && (!containerNameInPlannedSet(provider.Kind(), container, project, plannedNames) || container.Running) {
				listErr = errors.Join(listErr, errors.New("selected stop observed an unexpected or running service process"))
				r.VerificationFailures = append(r.VerificationFailures, "unexpected-or-running-selected-process:"+container.Service)
			}
		}
	}
	for name, prior := range baseline {
		current, exists := observed[name]
		if !exists && selected[prior.Service] {
			// The existing native selected-stop adapter may remove its stopped
			// containers. This is process retirement, never data deletion. Exact
			// resource absence and replica inspection above must both succeed.
			r.Stopped = append(r.Stopped, ProjectResource{Kind: "container", Name: project + "-" + prior.Service + "-1"})
			continue
		}
		if !exists || current.ID != prior.ID || current.Project != prior.Project || current.Service != prior.Service {
			r.VerificationFailures = append(r.VerificationFailures, "retained-process-owner-changed:"+name)
			listErr = errors.Join(listErr, errors.New("selected stop changed retained native process ownership"))
			continue
		}
		if selected[prior.Service] {
			if current.Running || (current.State != "exited" && current.State != "stopped" && current.State != "created") {
				r.VerificationFailures = append(r.VerificationFailures, "selected-process-not-stopped:"+name)
				listErr = errors.Join(listErr, errors.New("selected service is not verified stopped"))
			} else {
				r.Stopped = append(r.Stopped, ProjectResource{Kind: "container", Name: project + "-" + prior.Service + "-1"})
			}
		} else if current.Running != prior.Running || current.State != prior.State {
			r.VerificationFailures = append(r.VerificationFailures, "sibling-process-changed:"+name)
			listErr = errors.Join(listErr, errors.New("selected stop changed a sibling service"))
		}
	}
	r.Retained = after
	err = errors.Join(mutationErr, inspectErr, listErr)
	r.Verified = err == nil
	return r, err
}
