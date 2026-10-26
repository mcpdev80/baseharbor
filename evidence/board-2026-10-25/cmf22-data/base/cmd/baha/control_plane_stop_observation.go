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
	for _, container := range containers {
		if container.Project != p.Project {
			continue
		}
		p.ObservedProcesses = append(p.ObservedProcesses, coreStopProcessObservation{ID: container.ID, Name: container.Name, Service: container.Service, Running: container.Running, State: container.State})
		if selected[container.Service] {
			if !bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, p.Project, bhruntime.ProjectResource{Kind: "container", Name: p.Project + "-" + container.Service + "-1"}) {
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
