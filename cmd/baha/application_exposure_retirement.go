package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func applicationExposureInstance(instance capability.ProviderInstance, m application.Manifest) bool {
	return instance.Provider.Kind == capability.ProviderCaddy && instance.Scope == capability.ScopeApplication && instance.Ownership == capability.OwnershipBaseHarbor && instance.OwnerApplicationID == m.ApplicationID && instance.OwnerEnvironment == m.Environment
}

func observeApplicationExposureFacts(ctx context.Context, resolved resolvedApplication, facts map[string]application.ProviderFact) {
	if isRemoteApplication(resolved) {
		return
	}
	inventory, err := exposure.ExistingRuntimeInventory(resolved.Manifest, application.RuntimeFilesFor(resolved.Store, resolved.Manifest))
	if err != nil {
		return
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return
	}
	if _, err := provider.InspectProjectResources(ctx, inventory.State.Project, inventory.Resources); err != nil {
		return
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return
	}
	count, state := int64(0), application.ProviderStoppedOwned
	for _, container := range containers {
		for _, resource := range inventory.Resources {
			if bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, inventory.State.Project, resource) {
				count++
				if container.Running {
					state = application.ProviderRunning
				}
				break
			}
		}
	}
	for _, instance := range registry.Instances {
		if applicationExposureInstance(instance, resolved.Manifest) {
			facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: application.CoreNotRequired, Dependencies: []string{}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "protected canonical exposure renderer and native ownership inventory"}, MemoryBytes: application.Quantity{Classification: "unknown"}}
		}
	}
}

func retireApplicationExposureRuntime(ctx context.Context, resolved resolvedApplication, provider bhruntime.RuntimeProvider, instance capability.ProviderInstance, plan application.ProviderPlan, out io.Writer) error {
	if !applicationExposureInstance(instance, resolved.Manifest) || len(resolved.Manifest.Exposures) != 0 || !plan.Complete || plan.Target != resolved.Target.Name || plan.Version != application.ProviderPlanVersion {
		return invalidProviderConsumerInventory(fmt.Errorf("exposure retirement requires its complete unused application owner plan"))
	}
	runtime := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	files := exposure.FilesFor(runtime)
	missing, err := missingExposureProjection(runtime.Dir, files.Dir)
	if err != nil {
		return fmt.Errorf("inspect application exposure owner: %w", err)
	}
	if missing {
		// Development Targets publish through the target-wide gateway and do
		// not necessarily materialize a per-application Caddy project. Once the
		// canonical application route is gone, retire only any remaining
		// explicitly owned Caddy containers discovered by project identity.
		if !devaccess.Enabled(resolved.Manifest.Environment) {
			return fmt.Errorf("application exposure owner state is missing outside a development Target")
		}
		owner := "app/" + resolved.Manifest.Name + "/" + resolved.Manifest.Environment
		present, routeErr := devgateway.OwnerRoutesPresent(ctx, provider, resolved.Target.Name, owner)
		if routeErr != nil {
			return fmt.Errorf("verify development route before exposure retirement: %w", routeErr)
		}
		if present {
			return fmt.Errorf("application development route remains registered; remove it before exposure retirement")
		}
		project := exposure.ProjectNameForRuntime(resolved.Manifest, runtime)
		resources, listErr := provider.ListOwnedProjectResources(ctx, project)
		if listErr != nil {
			return fmt.Errorf("inspect owned application exposure resources: %w", listErr)
		}
		retirement, ok := provider.(bhruntime.RuntimeServiceRetirementProvider)
		if !ok && len(resources) > 0 {
			return fmt.Errorf("runtime cannot safely retire orphaned application exposure containers")
		}
		removed, retireErr := retireOrphanedExposureResources(ctx, retirement, project, resources)
		for _, item := range removed {
			fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", item.Kind, item.Name)
		}
		if retireErr != nil {
			return fmt.Errorf("retire orphaned application exposure containers: %w", retireErr)
		}
		return nil
	}
	result, err := exposure.RetireRuntime(ctx, provider, resolved.Manifest, runtime)
	for _, item := range result.Removed {
		fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", item.Kind, item.Name)
	}
	for _, item := range result.Retained {
		fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
	}
	return err
}

func missingExposureProjection(runtimeDir, exposureDir string) (bool, error) {
	for _, path := range []string{runtimeDir, filepath.Join(runtimeDir, "providers"), exposureDir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return false, fmt.Errorf("application exposure owner directory %s is unprotected or redirected", path)
		}
	}
	return false, nil
}

func retireOrphanedExposureResources(ctx context.Context, provider bhruntime.RuntimeServiceRetirementProvider, project string, resources []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	remove := make([]bhruntime.ProjectResource, 0, len(resources))
	prefix := project + "-baseharbor-internal-exposure-"
	for _, resource := range resources {
		if resource.Kind != "container" || !strings.HasPrefix(resource.Name, prefix) || !strings.HasSuffix(resource.Name, "-1") {
			continue
		}
		remove = append(remove, resource)
	}
	if len(remove) == 0 {
		return []bhruntime.ProjectResource{}, nil
	}
	if provider == nil {
		return nil, fmt.Errorf("runtime cannot safely retire orphaned application exposure containers")
	}
	result, err := bhruntime.RetireOwnedServices(ctx, provider, project, resources, remove)
	return result.Removed, err
}
