package main

import (
	"context"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtimeexplorer"
)

func (e *bahaMachineExecutor) OpenLogStream(ctx context.Context, request machine.StreamRequest) (io.ReadCloser, error) {
	explorer, target, err := runtimeExplorerForTarget(ctx, request.Context.Target)
	if err != nil {
		return nil, err
	}
	capabilities, err := explorer.Capabilities(ctx, target)
	if err != nil {
		return nil, err
	}
	return explorer.Logs(ctx, runtimeexplorer.LogRequest{
		Resource: runtimeexplorer.ResourceRef{
			Provider:   capabilities.Provider,
			Target:     target,
			Kind:       runtimeexplorer.ResourceKind(strings.TrimSpace(request.ResourceKind)),
			ResourceID: strings.TrimSpace(request.ResourceID),
		},
		Since:  request.Since,
		Tail:   request.Tail,
		Follow: request.Follow,
	})
}

func (e *bahaMachineExecutor) OpenExecStream(ctx context.Context, request machine.StreamRequest) (io.ReadCloser, error) {
	if request.TTY {
		return nil, machine.NewError(
			machine.ErrorUnsupported,
			"Interactive TTY exec is not supported by the active Runtime Explorer provider.",
			"Use a non-interactive bounded command.",
			false,
		)
	}
	explorer, target, err := runtimeExplorerForTarget(ctx, request.Context.Target)
	if err != nil {
		return nil, err
	}
	capabilities, err := explorer.Capabilities(ctx, target)
	if err != nil {
		return nil, err
	}
	return explorer.Exec(ctx, runtimeexplorer.OperationRequest{
		Resource: runtimeexplorer.ResourceRef{
			Provider:   capabilities.Provider,
			Target:     target,
			Kind:       runtimeexplorer.ResourceKind(strings.TrimSpace(request.ResourceKind)),
			ResourceID: strings.TrimSpace(request.ResourceID),
		},
		Operation: runtimeexplorer.OperationExec,
		Command:   append([]string(nil), request.Command...),
	})
}

var _ interface {
	OpenLogStream(context.Context, machine.StreamRequest) (io.ReadCloser, error)
	OpenExecStream(context.Context, machine.StreamRequest) (io.ReadCloser, error)
} = (*bahaMachineExecutor)(nil)
