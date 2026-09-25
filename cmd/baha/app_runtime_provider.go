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
		provider = bhruntime.ProviderCompose
	}
	return bhruntime.ParseProviderKind(string(provider))
}

// detectComposeForApplication is the transitional adapter used while v0.4
// moves existing Compose orchestration behind the provider seam incrementally.
// Future providers must not be coerced into Compose behavior: selection and
// capability checks happen before the current Compose-only operation proceeds.
func detectComposeForApplication(ctx context.Context, resolved resolvedApplication, required ...bhruntime.RuntimeCapability) (bhruntime.Compose, error) {
	kind, err := runtimeProviderKindForApplication(resolved)
	if err != nil {
		return bhruntime.Compose{}, err
	}
	provider, err := bhruntime.DetectProviderForKind(ctx, kind)
	if err != nil {
		return bhruntime.Compose{}, err
	}
	if err := bhruntime.RequireCapabilities(provider, required...); err != nil {
		return bhruntime.Compose{}, err
	}
	compose, ok := provider.(bhruntime.Compose)
	if !ok {
		return bhruntime.Compose{}, fmt.Errorf("runtime provider %s is not implemented for Compose-backed orchestration", provider.Kind())
	}
	return compose, nil
}
