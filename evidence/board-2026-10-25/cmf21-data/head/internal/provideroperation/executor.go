package provideroperation

import (
	"context"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type Executor struct {
	runtime     bhruntime.RuntimeProvider
	project     string
	composeFile string
	envFile     string
}

func New(runtime bhruntime.RuntimeProvider, project, composeFile, envFile string) Executor {
	return Executor{
		runtime:     runtime,
		project:     project,
		composeFile: composeFile,
		envFile:     envFile,
	}
}

func (e Executor) Run(ctx context.Context, service string, args ...string) (string, error) {
	return e.runtime.ExecProject(ctx, e.project, e.composeFile, e.envFile, service, args...)
}

func (e Executor) RunSensitive(ctx context.Context, service string, input []byte, args ...string) (string, error) {
	return e.runtime.ExecProjectInput(ctx, e.project, e.composeFile, e.envFile, input, service, args...)
}
