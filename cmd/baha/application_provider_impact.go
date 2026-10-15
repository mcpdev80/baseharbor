package main

import (
	"context"
	"reflect"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Project the existing union actions into consequence requests. This selects
// no providers and creates no independent dependency/removal policy.
func applicationProviderChangeRequests(plan application.ProviderPlan) []application.ProviderChangeRequest {
	requests := []application.ProviderChangeRequest{}
	for _, change := range plan.Changes {
		operation := change.Action
		switch operation {
		case "add", "stop", "remove-runtime":
		case "restart":
			operation = "resume"
		default:
			continue
		}
		requests = append(requests, application.ProviderChangeRequest{InstanceID: change.InstanceID, Operation: operation, Selection: "automatic", Consumers: []string{}, Materials: []application.MaterialDisposition{}})
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].InstanceID < requests[j].InstanceID })
	return requests
}

// Updating a retained realization can remove individual named services or
// management access without releasing the whole provider. Include those actual
// renderer changes in the same acknowledged consequence plan.
func nativeBackendChangeRequests(resolved resolvedApplication, existing []application.ProviderChangeRequest) ([]application.ProviderChangeRequest, error) {
	prior, err := retainedApplicationBackendIntent(resolved)
	if err != nil {
		return nil, err
	}
	registry, err := application.PreviewReferenceProviderRegistryAt(resolved.TargetStateRoot, resolved.Manifest)
	if err != nil {
		return nil, err
	}
	files := application.RuntimeFilesFor(resolved.Store, resolved.Manifest)
	seen := map[string]bool{}
	for _, request := range existing {
		seen[request.InstanceID] = true
	}
	for _, instance := range registry.Instances {
		if seen[instance.ID] || !applicationNativeBackendInstance(instance, resolved.Manifest) {
			continue
		}
		before, err := application.NativeBackendInstanceResources(prior, files, instance)
		if err != nil {
			return nil, err
		}
		after, err := application.NativeBackendInstanceResources(resolved.Manifest, files, instance)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(before, after) {
			existing = append(existing, application.ProviderChangeRequest{InstanceID: instance.ID, Operation: "update", Selection: "automatic", Consumers: []string{}, Materials: []application.MaterialDisposition{}})
		}
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].InstanceID < existing[j].InstanceID })
	return existing, nil
}

func resolveApplicationProviderImpact(ctx context.Context, resolved resolvedApplication, snapshot application.ResolutionSnapshot, union application.ProviderPlan, requests []application.ProviderChangeRequest) (application.ProviderImpactPlan, error) {
	observation := observeApplicationProviderImpact(ctx, resolved, snapshot)
	appendStandaloneVersionChangeEffects(ctx, resolved, requests, &observation)
	if err := appendExternalReceiverChangeEffects(resolved, &observation); err != nil {
		return application.ProviderImpactPlan{}, err
	}
	if err := appendExternalIdentityChangeEffects(resolved, &observation); err != nil {
		return application.ProviderImpactPlan{}, err
	}
	// Impact is derived presentation metadata, not another authoritative input.
	// Exclude an earlier attachment from the plan-bound revalidation hash.
	union.Impact = nil
	plan, err := application.DescribeProviderChanges(snapshot, union, requests, observation)
	return plan, classifyProviderChangeConsent(err)
}
