package resolver

import (
	"context"
	"fmt"

	dockerprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/docker"
	kubernetesprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/kubernetes"
	podmanprovider "github.com/mcpdev80/baseharbor/internal/providers/runtime/podman"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

var registry = mustRegistry(
	runtimecontract.ProviderRegistration{
		Descriptor: dockerprovider.Descriptor(),
		Factory: func(ctx context.Context) (runtimecontract.Provider, error) {
			return dockerprovider.New(ctx)
		},
	},
	runtimecontract.ProviderRegistration{
		Descriptor: podmanprovider.Descriptor(),
		Factory: func(ctx context.Context) (runtimecontract.Provider, error) {
			return podmanprovider.New(ctx)
		},
	},
	runtimecontract.ProviderRegistration{
		Descriptor: (kubernetesprovider.Provider{}).Descriptor(),
		Factory: func(ctx context.Context) (runtimecontract.Provider, error) {
			provider, err := kubernetesprovider.Detect(ctx)
			if err != nil {
				return nil, err
			}
			return provider, nil
		},
	},
)

func mustRegistry(registrations ...runtimecontract.ProviderRegistration) *runtimecontract.ProviderRegistry {
	registry, err := runtimecontract.NewProviderRegistry(registrations...)
	if err != nil {
		panic(err)
	}
	return registry
}

func Descriptor(kind runtimecontract.ProviderKind) (runtimecontract.ProviderDescriptor, error) {
	return registry.Descriptor(kind)
}

func Provider(ctx context.Context, kind runtimecontract.ProviderKind) (runtimecontract.Provider, error) {
	return registry.Resolve(ctx, kind)
}

func RuntimeProvider(ctx context.Context, kind runtimecontract.ProviderKind) (runtimecontract.RuntimeProvider, error) {
	return registry.ResolveRuntimeProvider(ctx, kind)
}

func WorkloadProvider(ctx context.Context, kind runtimecontract.ProviderKind) (runtimecontract.InternalWorkloadProvider, error) {
	provider, err := registry.Resolve(ctx, kind)
	if err != nil {
		return nil, err
	}
	workloadProvider, ok := provider.(runtimecontract.InternalWorkloadProvider)
	if !ok {
		return nil, fmt.Errorf("runtime provider %q does not implement the workload lifecycle contract", provider.Kind())
	}
	return workloadProvider, nil
}

func DefaultRuntimeProvider(ctx context.Context) (runtimecontract.RuntimeProvider, error) {
	return RuntimeProvider(ctx, runtimecontract.ProviderDocker)
}
