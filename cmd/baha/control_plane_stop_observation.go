package main

import (
	"context"
	"sort"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type coreStopProcessObservation struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Service string `json:"service"`
	Running bool   `json:"running"`
	State   string `json:"state"`
}

func observeControlPlaneStopRuntime(parent context.Context, target deployment.ResolvedTarget, p *controlPlaneStopPlan) error {
	p.ObservedProcesses = []coreStopProcessObservation{}
	p.ObservedResources = []bhruntime.ProjectResource{}
	p.nativeRevision = "unverified"
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return nil
	}
	resources, err := provider.InspectProjectResources(ctx, p.Project, p.Resources)
	if err != nil {
		return runtimeMutationOwnerError()
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return nil
	}
	selected := map[string]bool{}
	for _, service := range p.Services {
		selected[service] = true
	}
	images := map[string]any{}
	// Pinned container_name lives outside <project>-<service>-1, so the control
	// plane accepts both the canonical ordinal and the rendered planned native
	// name for each selected service. The canonical name still owns the
	// service; a matching native name alone never infers ownership
	// (0427-128: control-plane stop accepts both identity classes). This check
	// runs before any unconditional canonical-only rejection so a pinned name is
	// admitted here and never dropped by an early guard.
	plannedByService := map[string][]string{}
	if configurer, ok := provider.(interface {
		ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error)
	}); ok {
		if rendered, rerr := configurer.ConfigJSONProjectFilesEnv(ctx, p.Project, "", nil); rerr == nil && rendered != "" {
			for service := range selected {
				if planned, perr := bhruntime.ParsedExpectedContainerNames(rendered, p.Project, service); perr == nil {
					plannedByService[service] = planned
				}
			}
		}
	}
	for _, container := range containers {
		if container.Project != p.Project {
			continue
		}
		p.ObservedProcesses = append(p.ObservedProcesses, coreStopProcessObservation{ID: container.ID, Name: container.Name, Service: container.Service, Running: container.Running, State: container.State})
		if selected[container.Service] {
			plannedNames := plannedByService[container.Service]
			if len(plannedNames) == 0 {
				// No rendered model: fall back to the single canonical ordinal
				// so the pre-existing canonical-owner contract is preserved.
				plannedNames = []string{p.Project + "-" + container.Service + "-1"}
			}
			if !bhruntime.ContainerNameInPlannedSet(provider.Kind(), container, p.Project, plannedNames) {
				return runtimeMutationOwnerError()
			}
			image, imageErr := provider.ProjectServiceImageIdentity(ctx, p.Project, container.Service)
			if imageErr == nil {
				images[container.Service] = image
			} else {
				images[container.Service] = "unverified"
			}
		}
	}
	sort.Slice(p.ObservedProcesses, func(i, j int) bool { return p.ObservedProcesses[i].Name < p.ObservedProcesses[j].Name })
	p.ObservedResources = resources
	p.RuntimeObserved = true
	// Aggregate identity revision only; never expose individual image data or
	// native error/inspect output through a consequence result.
	p.nativeRevision = runtimeMutationRevision(images)
	return nil
}
