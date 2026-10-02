package runtimeexplorer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type DirectContainerRuntime interface {
	Kind() runtimecontract.ProviderKind
	ListRuntimeContainers(context.Context) ([]runtimecontract.RuntimeContainer, error)
	DirectOutput(context.Context, ...string) (string, error)
}

type CLIContainerBackend struct {
	runtime DirectContainerRuntime
}

func NewCLIContainerBackend(runtime DirectContainerRuntime) (*CLIContainerBackend, error) {
	if runtime == nil {
		return nil, errors.New("direct container runtime is required")
	}
	return &CLIContainerBackend{runtime: runtime}, nil
}

func (b *CLIContainerBackend) Kind() runtimecontract.ProviderKind {
	return b.runtime.Kind()
}

func (b *CLIContainerBackend) ListRuntimeContainers(ctx context.Context) ([]runtimecontract.RuntimeContainer, error) {
	return b.runtime.ListRuntimeContainers(ctx)
}

func (b *CLIContainerBackend) ContainerLogs(ctx context.Context, id string, since *time.Time, tail int) (io.ReadCloser, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("runtime container id is required")
	}
	args := []string{"container", "logs"}
	if since != nil {
		args = append(args, "--since", since.UTC().Format(time.RFC3339))
	}
	if tail > 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}
	args = append(args, id)
	output, err := b.runtime.DirectOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(output)), nil
}

func (b *CLIContainerBackend) OperateContainer(ctx context.Context, id string, operation Operation, command []string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("runtime container id is required")
	}
	switch operation {
	case OperationStart:
		return b.runtime.DirectOutput(ctx, "container", "start", id)
	case OperationStop:
		return b.runtime.DirectOutput(ctx, "container", "stop", id)
	case OperationRestart:
		return b.runtime.DirectOutput(ctx, "container", "restart", id)
	case OperationExec:
		if len(command) == 0 {
			return "", errors.New("runtime exec requires an explicit command")
		}
		args := append([]string{"container", "exec", id}, command...)
		return b.runtime.DirectOutput(ctx, args...)
	default:
		return "", fmt.Errorf("unsupported runtime operation %q", operation)
	}
}

var _ ContainerBackend = (*CLIContainerBackend)(nil)
