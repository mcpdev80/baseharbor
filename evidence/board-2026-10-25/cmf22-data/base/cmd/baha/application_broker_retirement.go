package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

func retireUnneededApplicationBroker(ctx context.Context, resolved resolvedApplication, provider bhruntime.RuntimeProvider, plan application.ProviderPlan, out io.Writer) error {
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return err
	}
	if !application.RequiresRuntimeBroker(prior) || application.RequiresRuntimeBroker(resolved.Manifest) {
		return nil
	}
	if plan.Version != application.ProviderPlanVersion || plan.Target != resolved.Target.Name || !plan.Complete {
		return invalidProviderConsumerInventory(fmt.Errorf("broker retirement requires complete consumer inventory"))
	}
	instance := application.RuntimeBrokerDependency(resolved.Target.Name, prior)
	selected := false
	for _, change := range plan.Changes {
		if change.InstanceID == instance.ID && (change.Action == "stop" || change.Action == "remove-runtime") && len(change.Consumers) == 0 {
			selected = true
		}
	}
	if !selected {
		return invalidProviderConsumerInventory(fmt.Errorf("broker removal is absent from the acknowledged consumer plan"))
	}
	result, err := runtimebroker.RetireContainer(ctx, provider, prior, application.RuntimeFilesFor(resolved.Store, prior))
	for _, resource := range result.Removed {
		fmt.Fprintf(out, "Removed %s %s after owned runtime inspection\n", resource.Kind, resource.Name)
	}
	for _, item := range result.Retained {
		fmt.Fprintf(out, "Retained %s %s: %s\n", item.Resource.Kind, item.Resource.Name, item.Reason)
	}
	return err
}
