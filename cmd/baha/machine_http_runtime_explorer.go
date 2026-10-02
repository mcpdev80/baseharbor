package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

func (e *bahaMachineExecutor) OpenLogStream(ctx context.Context, request machine.StreamRequest) (io.ReadCloser, error) {
	targetName := strings.TrimSpace(request.Context.Target)
	ctx = withTargetOverride(ctx, targetName)
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	direct, ok := provider.(runtimeexplorer.DirectContainerRuntime)
	if !ok {
		return nil, fmt.Errorf("runtime provider %q does not expose bounded Runtime Explorer container primitives", provider.Kind())
	}
	backend, err := runtimeexplorer.NewCLIContainerBackend(direct)
	if err != nil {
		return nil, err
	}
	explorer, err := runtimeexplorer.NewService(backend, target.Name, deploymentRuntimeOwnershipResolver{})
	if err != nil {
		return nil, err
	}
	return explorer.Logs(ctx, runtimeexplorer.LogRequest{
		Resource: runtimeexplorer.ResourceRef{
			Provider:   string(provider.Kind()),
			Target:     target.Name,
			Kind:       runtimeexplorer.ResourceKind(strings.TrimSpace(request.ResourceKind)),
			ResourceID: strings.TrimSpace(request.ResourceID),
		},
		Since: request.Since,
		Tail:  request.Tail,
	})
}

var _ interface {
	OpenLogStream(context.Context, machine.StreamRequest) (io.ReadCloser, error)
} = (*bahaMachineExecutor)(nil)
