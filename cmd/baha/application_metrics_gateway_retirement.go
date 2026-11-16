package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/metrics"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func (e *applicationApplyExecution) loadRetiredSharedMetricsGatewayRoutes(ctx context.Context) ([]devgateway.OwnerRoutes, error) {
	routes, err := devgateway.Routes(e.resolved.Target.Name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	found := false
	for _, route := range routes {
		found = found || route.Owner == "shared/prometheus"
	}
	if !found {
		return nil, nil
	}
	plan := e.providerPlan
	if plan.Version == "" {
		plan, err = prepareApplicationProviderRetirement(ctx, e.resolved)
		if err != nil {
			return nil, err
		}
	}
	return retiredSharedMetricsGatewayRoutes(e.resolved, e.compose, plan, routes)
}

// Retained trust files do not keep a retired module routable. Use the complete
// acknowledged union and the same canonical owner as native retirement; an
// unrelated or still-required provider keeps its routes.
func retiredSharedMetricsGatewayRoutes(resolved resolvedApplication, provider bhruntime.RuntimeProvider, plan application.ProviderPlan, routes []devgateway.Route) ([]devgateway.OwnerRoutes, error) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(resolved.TargetStateRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	for _, instance := range registry.Instances {
		if instance.Provider.Kind != capability.ProviderPrometheus || instance.Scope != capability.ScopeShared {
			continue
		}
		selected := false
		for _, change := range plan.Changes {
			if change.InstanceID != instance.ID || (change.Action != "stop" && change.Action != "remove-runtime") || len(change.Consumers) != 0 {
				continue
			}
			if change.Ownership != capability.OwnershipBaseHarbor || instance.Ownership != capability.OwnershipBaseHarbor {
				return nil, invalidProviderConsumerInventory(fmt.Errorf("metrics route owner is not managed"))
			}
			selected = true
		}
		for _, required := range plan.Required {
			if required == instance.ID {
				selected = false
			}
		}
		if !selected {
			continue
		}
		if plan.Version != application.ProviderPlanVersion || plan.Target != resolved.Target.Name || !plan.Complete {
			return nil, invalidProviderConsumerInventory(fmt.Errorf("metrics route retirement requires its complete current Target plan"))
		}
		for _, binding := range registry.BindingsForProviderInstance(instance.ID) {
			released := false
			for _, release := range plan.ReleasedBindings {
				released = released || release == binding
			}
			if !released {
				return nil, invalidProviderConsumerInventory(fmt.Errorf("metrics route still has an unreleased consumer"))
			}
		}
		owners, err := applicationRuntimeOwners(resolved)
		if err != nil {
			return nil, err
		}
		for _, owner := range owners {
			inventory, err := existingStandaloneProviderInventory(owner, provider, instance, registry)
			if err != nil {
				continue
			}
			host, err := devaccess.SharedHost(resolved.Target.Name, "prometheus")
			if err != nil {
				return nil, err
			}
			expected := devgateway.Route{Owner: "shared/prometheus", Key: "shared/prometheus", Host: host, Upstream: "https://prometheus-access:9090", Network: metrics.PublishNetworkName(inventory.project), TrustFile: filepath.Join(inventory.dir, "service-access", "pki", "ca.pem"), ServerName: "prometheus"}
			found := false
			for _, route := range routes {
				if route.Owner != expected.Owner {
					continue
				}
				if route != expected {
					return nil, invalidProviderConsumerInventory(fmt.Errorf("metrics route differs from its canonical provider authority"))
				}
				found = true
			}
			if found {
				return []devgateway.OwnerRoutes{{Owner: expected.Owner}}, nil
			}
			return nil, nil
		}
		return nil, invalidProviderConsumerInventory(fmt.Errorf("metrics route has no retained canonical provider owner"))
	}
	return nil, nil
}
