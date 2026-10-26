package main

import (
	"context"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

type appUpOptions struct {
	Name                string
	Yes                 bool
	SkipMemoryPreflight bool
}

func parseAppUpOptions(args []string) (appUpOptions, error) {
	var opts appUpOptions
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			opts.Yes = true
		case "--skip-memory-preflight":
			opts.SkipMemoryPreflight = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, unknownOptionUsage("baha app up", arg, "--yes", "-y", "--skip-memory-preflight")
			}
			if opts.Name != "" {
				return opts, usageError("baha app up accepts at most one NAME", "Run 'baha app up --help' for usage.")
			}
			opts.Name = arg
		}
	}
	return opts, nil
}

func appUpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "up",
		Summary: "Start an existing application runtime and verify readiness",
		Usage:   "baha app up [NAME] [--yes|-y] [--skip-memory-preflight]",
		Long:    "Starts a previously materialized BaseHarbor application runtime using its existing runtime definition, credentials and persistent data; required application secrets are verified before workload start and missing values fail closed. Repository workloads are started after their BaseHarbor backend and per-application secret broker are ready. Workload-only applications skip the empty managed-runtime start and resume their repository Compose workload directly. Without NAME it resolves the nearest repository baseharbor.yaml. --yes accepts allowed confirmations but never invents unresolved selections or operator authentication configuration.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			opts, err := parseAppUpOptions(args)
			if err != nil {
				return err
			}
			ctx = withMemoryPreflightOverride(ctx, opts.SkipMemoryPreflight)
			ctx = withAssumeYes(ctx, opts.Yes)
			var lifecycleArgs []string
			if opts.Name != "" {
				lifecycleArgs = []string{opts.Name}
			}
			return executeApplicationUpLifecycle(ctx, store, lifecycleArgs, out, errOut)
		},
	}
}
