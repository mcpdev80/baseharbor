package resolver

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

var registry = mustRegistry(
	runtimecontract.ProviderRegistration{
		Descriptor: bhruntime.DockerProviderDescriptor(),
		Factory: func(ctx context.Context) (runtimecontract.Provider, error) {
			return bhruntime.NewDockerProvider(ctx)
		},
	},
	runtimecontract.ProviderRegistration{
		Descriptor: bhruntime.PodmanProviderDescriptor(),
		Factory: func(ctx context.Context) (runtimecontract.Provider, error) {
			return bhruntime.NewPodmanProvider(ctx)
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

func DefaultRuntimeProvider(ctx context.Context) (runtimecontract.RuntimeProvider, error) {
	return RuntimeProvider(ctx, runtimecontract.ProviderDocker)
}
