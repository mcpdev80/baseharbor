package main

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/capability"
	providerauthoring "github.com/mcpdev80/baseharbor/internal/provider/authoring"
)

func initializeProviderScaffold(ctx context.Context, root, id string) (providerauthoring.InitResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "provider.init", "", "", root); err != nil {
		return providerauthoring.InitResult{}, err
	}
	return providerauthoring.Init(root, id)
}
func inspectProviderContract(ctx context.Context, root string) (capability.ConformanceReport, error) {
	if err := authorizeCurrentMCPContext(ctx, "provider.test", "", "", root); err != nil {
		return capability.ConformanceReport{}, err
	}
	descriptor, err := providerauthoring.Load(root)
	if err != nil {
		return capability.ConformanceReport{}, err
	}
	return providerauthoring.Check(descriptor), nil
}
