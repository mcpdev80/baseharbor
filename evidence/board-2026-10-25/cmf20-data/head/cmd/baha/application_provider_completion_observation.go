package main

import (
	"context"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Native Podman may remove a completed one-shot container. Only its verified
// canonical systemd completion receipt can supply that missing observation.
func observeCanonicalProviderInitializers(ctx context.Context, provider interface{ Kind() bhruntime.ProviderKind }, project, compose, env string, services []string, containers []bhruntime.RuntimeContainer) (int64, bool) {
	total := int64(0)
	complete := true
	for _, service := range services {
		count, verified := observeCanonicalProviderCompletion(provider.Kind(), project, canonicalProviderServiceResources(project, []string{service}), containers)
		if count == 0 && provider.Kind() == bhruntime.ProviderPodman {
			if observer, ok := provider.(providerCompletionObserver); ok {
				receipt, err := observer.ObserveProjectServiceCompletion(ctx, project, compose, env, service)
				verified = err == nil && receipt
			}
		}
		total += count
		complete = complete && verified
	}
	return total, complete
}
