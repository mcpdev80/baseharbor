package main

import (
	"context"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// runtimeProviderKindForApplication resolves deployment-owned runtime selection.
// Named/legacy applications keep the Compose default until deployment metadata
// exists for those invocation paths as well.
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

// detectComposeForApplication is retained as a compatibility name while callers
// are migrated. It no longer exposes or requires a concrete Compose runtime.
func detectComposeForApplication(ctx context.Context, resolved resolvedApplication, required ...bhruntime.RuntimeCapability) (bhruntime.RuntimeProvider, error) {
	return detectRuntimeForApplication(ctx, resolved, required...)
}
