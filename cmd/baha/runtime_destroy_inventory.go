package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type targetDestroyInventory struct {
	Target   deployment.ResolvedTarget
	Runtime  bhruntime.RuntimeProvider
	Projects map[string][]bhruntime.ProjectResource
}

// Inventory uses the same provider ownership checks as execution. Names alone
// or volume creation times never authorize deletion.
func collectTargetDestroyInventory(ctx context.Context, target deployment.ResolvedTarget, sharedOnly bool) (targetDestroyInventory, error) {
	plan := targetDestroyInventory{Target: target, Projects: map[string][]bhruntime.ProjectResource{}}
	if strings.TrimSpace(target.RuntimeProvider) == "" {
		return plan, nil
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return plan, err
	}
	return collectTargetDestroyInventoryWithRuntime(ctx, target, provider, sharedOnly)
}

func collectTargetDestroyInventoryWithRuntime(ctx context.Context, target deployment.ResolvedTarget, provider bhruntime.RuntimeProvider, sharedOnly bool) (targetDestroyInventory, error) {
	plan := targetDestroyInventory{Target: target, Runtime: provider, Projects: map[string][]bhruntime.ProjectResource{}}
	projects := map[string]bool{bhruntime.SharedProjectName(target.Name): true}
	if !sharedOnly {
		containers, err := provider.ListRuntimeContainers(ctx)
		if err != nil {
			return plan, err
		}
		for _, container := range targetOwnedRuntimeContainers(target.Name, containers) {
			projects[container.Project] = true
		}
	}
	for project := range projects {
		resources, err := provider.ListOwnedProjectResources(ctx, project)
		if err != nil {
			return plan, fmt.Errorf("inventory project %s: %w", project, err)
		}
		sort.Slice(resources, func(i, j int) bool {
			if resources[i].Kind != resources[j].Kind {
				return resources[i].Kind < resources[j].Kind
			}
			return resources[i].Name < resources[j].Name
		})
		plan.Projects[project] = resources
	}
	return plan, nil
}

func (plan targetDestroyInventory) projectNames() []string {
	names := make([]string, 0, len(plan.Projects))
	for name := range plan.Projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (plan targetDestroyInventory) render(out io.Writer) {
	for _, project := range plan.projectNames() {
		for _, resource := range plan.Projects[project] {
			fmt.Fprintf(out, "  owned resource: target=%s project=%s %s %s\n", plan.Target.Name, project, resource.Kind, resource.Name)
		}
	}
	fmt.Fprintln(out, "  owned container anonymous volumes: remove with owned containers when unshared; named/external repository data is preserved")
}

func (plan targetDestroyInventory) cleanup(ctx context.Context, results *[]fullDestroyResult) {
	if plan.Runtime == nil {
		return
	}
	for _, project := range plan.projectNames() {
		resources := plan.Projects[project]
		// Module cleanup may already have removed resources. The provider checks
		// current ownership again and treats an absent resource as already removed.
		if err := plan.Runtime.DestroyOwnedProjectResources(ctx, project, resources); err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: plan.Target.Name, Resource: "planned resources " + project, Detail: err.Error()})
			continue
		}
		remaining, err := plan.Runtime.ListOwnedProjectResources(ctx, project)
		if err != nil {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: plan.Target.Name, Resource: "resource audit " + project, Detail: err.Error()})
			continue
		}
		set := map[bhruntime.ProjectResource]bool{}
		for _, resource := range remaining {
			set[resource] = true
		}
		for _, resource := range resources {
			status := "REMOVED"
			if set[resource] {
				status = "FAILED"
			}
			*results = append(*results, fullDestroyResult{Status: status, Target: plan.Target.Name, Resource: resource.Kind + " " + resource.Name, Detail: "project=" + project})
		}
		for _, resource := range remaining {
			*results = append(*results, fullDestroyResult{Status: "FAILED", Target: plan.Target.Name, Resource: "residual " + resource.Kind + " " + resource.Name, Detail: "owned resource remains; target state preserved"})
		}
	}
}

func preservedTargetRecovery(ctx context.Context, target deployment.ResolvedTarget) (fullDestroyResult, error) {
	path, err := defaultTargetRecoveryFile(target.Name)
	if err != nil {
		return fullDestroyResult{}, err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return fullDestroyResult{}, err
	}
	if definition, found := cfg.Targets[target.Name]; found && strings.TrimSpace(definition.OpenBao.RecoveryFile) != "" {
		path, err = filepath.Abs(definition.OpenBao.RecoveryFile)
		if err != nil {
			return fullDestroyResult{}, err
		}
	}

	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return fullDestroyResult{}, nil
	} else if err != nil {
		return fullDestroyResult{}, err
	}
	return fullDestroyResult{Status: "PRESERVED", Target: target.Name, Resource: "external recovery file", Detail: path + "; operator recovery material is outside installation state; remove deliberately only after deciding it is no longer needed"}, nil
}

func collectFullDestroyInventory(ctx context.Context, targets []deployment.ResolvedTarget) ([]targetDestroyInventory, error) {
	inventoryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	plans := make([]targetDestroyInventory, 0, len(targets))
	for _, target := range targets {
		plan, err := collectTargetDestroyInventory(inventoryCtx, target, false)
		if err != nil {
			return nil, fmt.Errorf("full destroy inventory for target %s: %w", target.Name, err)
		}
		plans = append(plans, plan)
	}
	return plans, nil
}
