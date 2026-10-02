package openbao

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// CommandExecutor is the runtime-neutral execution seam for OpenBao semantics.
// It executes commands in the already-selected OpenBao provider instance.
// Provider placement and runtime-native addressing remain outside this contract.
type CommandExecutor interface {
	Exec(context.Context, ...string) (string, error)
	ExecInput(context.Context, []byte, ...string) (string, error)
}

type runtimeCommandExecutor struct {
	executor Executor
	files    bhruntime.Files
}

func NewCommandExecutor(executor Executor, files bhruntime.Files) CommandExecutor {
	return runtimeCommandExecutor{executor: executor, files: files}
}

func (e runtimeCommandExecutor) Exec(ctx context.Context, args ...string) (string, error) {
	return e.executor.ExecProject(
		ctx,
		projectNameForFiles(e.files),
		e.files.Compose,
		e.files.Env,
		serviceName,
		args...,
	)
}

func (e runtimeCommandExecutor) ExecInput(ctx context.Context, input []byte, args ...string) (string, error) {
	return e.executor.ExecProjectInput(
		ctx,
		projectNameForFiles(e.files),
		e.files.Compose,
		e.files.Env,
		input,
		serviceName,
		args...,
	)
}

// ExecutorFromCommand adapts a runtime-neutral OpenBao executor to the legacy
// project-shaped boundary. The project/compose/env/service arguments are
// intentionally ignored: provider realization selected the execution target
// before OpenBao semantics are invoked.
func ExecutorFromCommand(executor CommandExecutor) Executor {
	return commandExecutorAdapter{executor: executor}
}

type commandExecutorAdapter struct {
	executor CommandExecutor
}

func (e commandExecutorAdapter) ExecProject(ctx context.Context, _, _, _, _ string, args ...string) (string, error) {
	return e.executor.Exec(ctx, args...)
}

func (e commandExecutorAdapter) ExecProjectInput(ctx context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	return e.executor.ExecInput(ctx, input, args...)
}
