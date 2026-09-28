package main

import (
	"context"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// runtimeProviderKindForApplication resolves deployment-owned runtime selection.
func runtimeProviderKindForApplication(resolved resolvedApplication) (bhruntime.ProviderKind, error) {
	provider := bhruntime.ProviderKind(resolved.Target.RuntimeProvider)
	if provider == "" {
		return "", fmt.Errorf("target %q has no runtime provider", resolved.Target.Name)
	}
	return bhruntime.ParseProviderKind(string(provider))
}

// detectRuntimeForApplication resolves the target-selected runtime through the
// provider-neutral execution contract and validates required runtime capabilities.
func detectRuntimeForApplication(ctx context.Context, resolved resolvedApplication, required ...bhruntime.RuntimeCapability) (bhruntime.RuntimeProvider, error) {
	kind, err := runtimeProviderKindForApplication(resolved)
	if err != nil {
		return nil, err
	}
	provider, err := bhruntime.ResolveRuntimeProviderForKind(ctx, kind)
	if err != nil {
		return nil, err
	}
	if err := bhruntime.RequireCapabilities(provider, required...); err != nil {
		return nil, err
	}
	return provider, nil
}
