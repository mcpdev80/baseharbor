package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// A single engine listing can transiently omit existing containers while
// unrelated lifecycle work races with it. A zero count must never become a
// stopped verdict from one sample: confirm with one fresh listing before the
// consequence plan (and its consent digest) changes shape.
func observeCanonicalProviderContainersConfirmed(ctx context.Context, lister interface {
	ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error)
}, kind bhruntime.ProviderKind, project string, resources []bhruntime.ProjectResource, containers []bhruntime.RuntimeContainer) (int64, application.ProviderState) {
	count, state := observeCanonicalProviderContainers(kind, project, resources, containers)
	if count == 0 && state == application.ProviderStoppedOwned {
		if fresh, err := lister.ListRuntimeContainers(ctx); err == nil {
			count, state = observeCanonicalProviderContainers(kind, project, resources, fresh)
		}
	}
	return count, state
}

// Observe the complete selected canonical module, not the first running member.
// The caller proves ownership separately. Running is process/health evidence,
// never proof of credentials, data, end-user access or HA quorum.
func observeCanonicalProviderContainers(kind bhruntime.ProviderKind, project string, resources []bhruntime.ProjectResource, containers []bhruntime.RuntimeContainer) (int64, application.ProviderState) {
	wanted := map[string]bool{}
	for _, resource := range resources {
		if resource.Kind == "container" {
			if wanted[resource.Name] {
				return 0, application.ProviderUnverifiable
			}
			wanted[resource.Name] = true
		}
	}
	if len(wanted) == 0 {
		return 0, application.ProviderUnverifiable
	}
	count, ready := int64(0), 0
	ambiguous, running := false, false
	for name := range wanted {
		matches := 0
		for _, container := range containers {
			if !bhruntime.ContainerMatchesProjectResource(kind, container, project, bhruntime.ProjectResource{Kind: "container", Name: name}) {
				continue
			}
			matches++
			count++
			running = running || container.Running
			if container.Running && (container.Health == "" || container.Health == "healthy") {
				ready++
			}
		}
		ambiguous = ambiguous || matches > 1
	}
	if ambiguous {
		return count, application.ProviderUnverifiable
	}
	if ready == len(wanted) {
		return count, application.ProviderRunning
	}
	if running {
		return count, application.ProviderUnverifiable
	}
	return count, application.ProviderStoppedOwned
}

func canonicalProviderServiceResources(project string, services []string) []bhruntime.ProjectResource {
	resources := []bhruntime.ProjectResource{}
	for _, service := range services {
		resources = append(resources, bhruntime.ProjectResource{Kind: "container", Name: project + "-" + service + "-1"})
	}
	return resources
}

// A canonical service_completed_successfully prerequisite is available only
// with a unique, owned, actually completed container and successful exit status.
func observeCanonicalProviderCompletion(kind bhruntime.ProviderKind, project string, resources []bhruntime.ProjectResource, containers []bhruntime.RuntimeContainer) (int64, bool) {
	count := int64(0)
	complete := true
	for _, resource := range resources {
		matches, successful := 0, false
		for _, container := range containers {
			if !bhruntime.ContainerMatchesProjectResource(kind, container, project, resource) {
				continue
			}
			matches++
			count++
			successful = !container.Running && container.State == "exited" && container.ExitCode == 0 && container.Error == ""
		}
		complete = complete && matches == 1 && successful
	}
	return count, complete
}
