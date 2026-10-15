package runtimeexplorer

import "context"

func (b *CLIContainerBackend) nativeContainerID(ctx context.Context, id string) (string, error) {
	if supervised, ok := b.runtime.(supervisedContainerRuntime); ok {
		return supervised.ExplorerContainerNativeID(ctx, id)
	}
	return id, nil
}

func (b *CLIContainerBackend) ContainerResourceEvidence(ctx context.Context, id string) (map[string]string, error) {
	if source, ok := b.runtime.(interface {
		ExplorerContainerEvidence(context.Context, string) (map[string]string, error)
	}); ok {
		return source.ExplorerContainerEvidence(ctx, id)
	}
	return nil, nil
}
