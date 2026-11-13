package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type targetDestroyInventory struct {
	Target  deployment.ResolvedTarget
	Runtime bhruntime.RuntimeProvider
	// InventoryError means this Target could not be observed. Keep its retry
	// owner and state intact while allowing independently verified Targets to
	// proceed through the same acknowledged installation plan.
	InventoryError string
	Projects       map[string][]bhruntime.ProjectResource
	Volumes        map[string][]bhruntime.ContainerVolume
}

type destroyVolumeInventory interface {
	InventoryOwnedContainerVolumes(context.Context, string) ([]bhruntime.ContainerVolume, error)
	ContainerVolumeExists(context.Context, string) (bool, error)
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
	plan := targetDestroyInventory{Target: target, Runtime: provider, Projects: map[string][]bhruntime.ProjectResource{}, Volumes: map[string][]bhruntime.ContainerVolume{}}
	projects := map[string]bool{
		bhruntime.SharedProjectName(target.Name):           true,
		bhruntime.SharedProjectName(target.Name + "-core"): true,
	}
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
		if inventory, ok := provider.(destroyVolumeInventory); ok {
			volumes, err := inventory.InventoryOwnedContainerVolumes(ctx, project)
			if err != nil {
				return plan, fmt.Errorf("inventory mounted volumes for %s: %w", project, err)
			}
			ownedNamed := map[string]bool{}
			for _, resource := range resources {
				if resource.Kind == "volume" {
					ownedNamed[resource.Name] = true
				}
			}
			for _, volume := range volumes {
				if !ownedNamed[volume.Name] {
					plan.Volumes[project] = append(plan.Volumes[project], volume)
				}
			}
		}
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
	if plan.InventoryError != "" {
		fmt.Fprintf(out, "  PRESERVED target=%s (runtime inventory unavailable: %s)\n", plan.Target.Name, plan.InventoryError)
		return
	}
	for _, project := range plan.projectNames() {
		for _, resource := range plan.Projects[project] {
			fmt.Fprintf(out, "  owned resource: target=%s project=%s %s %s\n", plan.Target.Name, project, resource.Kind, resource.Name)
		}
		for _, volume := range plan.Volumes[project] {
			action := "PRESERVED"
			if volume.Removable {
				action = "REMOVE"
			}
			fmt.Fprintf(out, "  mounted volume: target=%s project=%s %s %s (%s)\n", plan.Target.Name, project, action, volume.Name, volume.Reason)
		}
	}
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
		if inventory, ok := plan.Runtime.(destroyVolumeInventory); ok {
			for _, volume := range plan.Volumes[project] {
				status, detail := "PRESERVED", volume.Reason
				if volume.Removable {
					exists, err := inventory.ContainerVolumeExists(ctx, volume.Name)
					status = "REMOVED"
					if err != nil || exists {
						status = "FAILED"
						detail = "owned anonymous volume removal could not be verified; target state preserved"
					}
				}
				*results = append(*results, fullDestroyResult{Status: status, Target: plan.Target.Name, Resource: "mounted volume " + volume.Name, Detail: detail})
			}
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
	path, err := targetRecoveryFilePath(target)
	if err != nil {
		return fullDestroyResult{}, err
	}

	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return fullDestroyResult{}, nil
	} else if err != nil {
		return fullDestroyResult{}, err
	}
	return fullDestroyResult{Status: "DELETE", Target: target.Name, Resource: "external recovery file", Detail: path + "; removed after the acknowledged installation destroy"}, nil
}

func targetRecoveryFilePath(target deployment.ResolvedTarget) (string, error) {
	path, err := defaultTargetRecoveryFile(target.Name)
	if err != nil {
		return "", err
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return "", err
	}
	if definition, found := cfg.Targets[target.Name]; found && strings.TrimSpace(definition.OpenBao.RecoveryFile) != "" {
		path, err = filepath.Abs(definition.OpenBao.RecoveryFile)
		if err != nil {
			return "", err
		}
	}
	return path, nil
}

func collectFullDestroyInventory(ctx context.Context, targets []deployment.ResolvedTarget) ([]targetDestroyInventory, error) {
	return collectFullDestroyInventoryUsing(ctx, targets, collectTargetDestroyInventory)
}

func collectFullDestroyInventoryUsing(ctx context.Context, targets []deployment.ResolvedTarget, collect func(context.Context, deployment.ResolvedTarget, bool) (targetDestroyInventory, error)) ([]targetDestroyInventory, error) {
	inventoryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	plans := make([]targetDestroyInventory, 0, len(targets))
	for _, target := range targets {
		plan, err := collect(inventoryCtx, target, false)
		if err != nil {
			plan = targetDestroyInventory{Target: target, Projects: map[string][]bhruntime.ProjectResource{}, Volumes: map[string][]bhruntime.ContainerVolume{}, InventoryError: err.Error()}
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func fullDestroyInventoryError(target deployment.ResolvedTarget, err error) error {
	if errors.Is(err, bhruntime.ErrRuntimeNotFound) {
		return machine.NewError(machine.ErrorCapabilityMissing,
			fmt.Sprintf("Cannot inspect Target %q because its %s runtime is unavailable on this workstation.", target.Name, target.RuntimeProvider),
			fmt.Sprintf("Restore or install the %s runtime used by Target %q, then run 'baha destroy' again. No resources were removed.", target.RuntimeProvider, target.Name), true)
	}
	return fmt.Errorf("full destroy inventory for target %s: %w", target.Name, err)
}
