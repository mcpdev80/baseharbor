package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func providerInstancePlacement(instance capability.ProviderInstance) capability.ProviderPlacement {
	return capability.ProviderPlacement{Scope: instance.Scope, Ownership: instance.Ownership, SharingBoundary: instance.SharingBoundary}
}

// Observe the exact owned Identity module even when the current application
// no longer requests Identity. Registry presence alone is never live evidence.
func observeIdentityProviderFacts(ctx context.Context, resolved resolvedApplication) map[string]application.ProviderFact {
	facts := map[string]application.ProviderFact{}
	if isRemoteApplication(resolved) {
		return facts
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil || len(registry.Instances) == 0 {
		return facts
	}
	provider, err := detectRuntimeForTarget(ctx, resolved.Target)
	if err != nil {
		return facts
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return facts
	}
	for _, instance := range registry.Instances {
		if instance.Provider.Kind != capability.ProviderKeycloak || instance.Ownership != capability.OwnershipBaseHarbor {
			continue
		}
		m := resolved.Manifest
		if instance.Scope == capability.ScopeApplication && instance.OwnerApplicationID != m.ApplicationID {
			m = application.Manifest{Version: application.CurrentVersion, ApplicationID: instance.OwnerApplicationID, Name: instance.OwnerApplication, Environment: instance.OwnerEnvironment}
		}
		files, expected, err := identityprovider.ExistingKeycloakRuntimeInventoryAt(m, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name)
		if err != nil {
			continue
		}
		if _, err := provider.InspectProjectResources(ctx, files.Project, expected); err != nil {
			continue
		}
		wanted := []bhruntime.ProjectResource{}
		for _, resource := range expected {
			if resource.Kind == "container" {
				wanted = append(wanted, resource)
			}
		}
		state, count := application.ProviderStoppedOwned, int64(0)
		sqlState, sqlCount := application.ProviderStoppedOwned, int64(0)
		for _, container := range containers {
			matched := false
			for _, resource := range wanted {
				matched = matched || bhruntime.ContainerMatchesProjectResource(provider.Kind(), container, files.Project, resource)
			}
			if matched {
				if strings.HasPrefix(container.Service, "keycloak-db-") || strings.HasPrefix(container.Name, files.Project+"-keycloak-db-") {
					sqlCount++
					if container.Running {
						sqlState = application.ProviderRunning
					}
					continue
				}
				count++
				if container.Running {
					state = application.ProviderRunning
				}
			}
		}
		sqlModule, sqlOwner, err := identityprovider.KeycloakSQLDependency(m, providerInstancePlacement(instance), resolved.TargetStateRoot, resolved.Target.Name, files.SharedSQL != nil)
		if err != nil {
			continue
		}
		dependencies := []string{sqlModule.ID}
		core := application.CoreNotRequired
		if files.SharedSQL != nil {
			core = application.CoreRequired
		} else {
			facts[sqlModule.ID] = application.ProviderFact{Target: resolved.Target.Name, State: sqlState, Core: application.CoreNotRequired, Dependencies: []string{}, Ancillary: true, OwnerReference: sqlOwner, MemoryBytes: application.Quantity{Classification: "unknown"}, Containers: application.Quantity{Value: &sqlCount, Classification: "measured", Source: "canonical owned embedded Keycloak PostgreSQL module"}}
		}
		facts[instance.ID] = application.ProviderFact{Target: resolved.Target.Name, State: state, Core: core, Dependencies: dependencies, MemoryBytes: application.Quantity{Classification: "unknown"}, Containers: application.Quantity{Value: &count, Classification: "measured", Source: "canonical Identity module and ownership-checked runtime container inventory"}}
	}
	return facts
}
