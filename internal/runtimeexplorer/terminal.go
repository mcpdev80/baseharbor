package runtimeexplorer

import (
	"context"
	"runtime"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
)

const CapabilityContainerTerminal Capability = "container.terminal"

type TerminalBackend interface {
	ContainerTerminal(context.Context, string, []string, int, int) (terminal.Session, error)
}

func (s *Service) Terminal(ctx context.Context, request OperationRequest, scope machine.OperationContext, rows, columns int) (terminal.Session, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Operation != OperationExec || request.Resource.Kind != KindContainer {
		return nil, machine.NewError(machine.ErrorUnsupported, "Terminal requires a container exec resource.", "Select an owned container from Runtime Explorer.", false)
	}
	if err := terminal.ValidateSize(rows, columns); err != nil {
		return nil, err
	}
	resource, err := s.Inspect(ctx, request.Resource)
	if err != nil {
		return nil, err
	}
	if resource.Ownership != OwnershipManaged && resource.Ownership != OwnershipPlatform {
		return nil, machine.NewError(machine.ErrorPolicyDenied, "Runtime resource ownership does not permit a terminal.", "Select a BaseHarbor-owned resource.", false)
	}
	if scope.Environment == "" || (resource.Relationship.Environment != "" && resource.Relationship.Environment != scope.Environment) || (scope.Application != "" && scope.Application != resource.Relationship.Application) {
		return nil, machine.NewError(machine.ErrorPolicyDenied, "Runtime resource is outside the authorized environment.", "Use the resource environment with the required operator authorization.", false)
	}
	backend, ok := s.backend.(TerminalBackend)
	if !ok {
		return nil, machine.NewError(machine.ErrorCapabilityMissing, "Runtime does not supply an interactive terminal.", "Negotiate the resource terminal capability.", false)
	}
	return backend.ContainerTerminal(ctx, request.Resource.ResourceID, request.Command, rows, columns)
}

func (b *CLIContainerBackend) ContainerTerminal(ctx context.Context, id string, argv []string, rows, columns int) (terminal.Session, error) {
	runtime, ok := b.runtime.(TerminalBackend)
	if !ok {
		return nil, machine.NewError(machine.ErrorCapabilityMissing, "Runtime does not supply an interactive terminal.", "Negotiate the resource terminal capability.", false)
	}
	return runtime.ContainerTerminal(ctx, id, argv, rows, columns)
}

func (b *CLIContainerBackend) TerminalAvailable() bool {
	_, ok := b.runtime.(TerminalBackend)
	return ok && runtime.GOOS == "linux"
}
