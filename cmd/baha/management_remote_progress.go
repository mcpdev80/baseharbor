package main

import (
	"context"
	"io"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

type remoteProgressKey struct{}
type remoteCLIProgress struct {
	out  io.Writer
	last string
}

func reportRemoteProgress(ctx context.Context, message string) {
	if message == "Operation accepted by BaseHarbor Core." && !cli.OutputOptionsFromContext(ctx).Verbose {
		return
	}
	if state, ok := ctx.Value(remoteProgressKey{}).(*remoteCLIProgress); ok && message != "" && state.last != message {
		state.last = message
		cli.ReportActivityDetail(state.out, message)
	}
}

func remoteCLIInteractive(ctx context.Context) bool {
	return !noInput(ctx) && readerIsTerminal(applicationInput(ctx, runtimeInput))
}

func remoteCLIPhase(ctx context.Context, out io.Writer, label string, run func(context.Context) error) error {
	if !remoteCLIInteractive(ctx) {
		return run(ctx)
	}
	opts := cli.OutputOptionsFromContext(ctx)
	opts.ProgressHeartbeat = -1
	renderCtx := cli.WithOutputOptions(ctx, opts)
	return cli.NewTerminal(renderCtx, out, out).Progress(renderCtx, label, func(progressOut io.Writer) error {
		return run(context.WithValue(ctx, remoteProgressKey{}, &remoteCLIProgress{out: progressOut}))
	})
}
