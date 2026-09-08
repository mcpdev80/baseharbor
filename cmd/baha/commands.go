package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/health"
)

func rootCommand() *cli.Command {
	store := application.DefaultStore()
	appCmd := appCommand(store)
	appCmd.Children = append(appCmd.Children,
		appApplyCommand(store),
		appStatusCommand(store),
		appDoctorCommand(store),
		appDownCommand(store),
		appUpCommand(store),
		appDestroyCommand(store),
	)

	root := &cli.Command{
		Name:    "baha",
		Summary: "BaseHarbor command-line interface",
		Usage:   "baha <command> [options]",
		Long:    "Manage BaseHarbor and isolated application backend runtimes from one binary. Commands are fail-closed: validation and preflight happen before mutation.",
	}

	root.Children = []*cli.Command{
		{
			Name:    "init",
			Summary: "Create a minimal BaseHarbor configuration",
			Usage:   "baha init",
			Run:     noArgs("baha init", initConfig),
		},
		{
			Name:    "up",
			Summary: "Start the local BaseHarbor control-plane runtime",
			Usage:   "baha up",
			Run:     noArgsCtx("baha up", runtimeUp),
		},
		{
			Name:    "down",
			Summary: "Stop the local BaseHarbor control-plane runtime",
			Usage:   "baha down",
			Run:     noArgsCtx("baha down", runtimeDown),
		},
		{
			Name:    "status",
			Summary: "Show control-plane container and readiness status",
			Usage:   "baha status",
			Run:     noArgsCtx("baha status", runtimeStatus),
		},
		{
			Name:    "doctor",
			Summary: "Verify host and control-plane prerequisites",
			Usage:   "baha doctor",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) != 0 {
					return usageError("baha doctor does not accept arguments", "Run 'baha doctor --help' for usage.")
				}
				formatted, ok := health.Format(health.Doctor())
				fmt.Fprint(out, formatted)
				if !ok {
					return errors.New("one or more checks failed")
				}
				return nil
			},
		},
		appCmd,
		{
			Name:    "version",
			Aliases: []string{"--version", "-v"},
			Summary: "Print build version",
			Usage:   "baha version",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) != 0 {
					return usageError("baha version does not accept arguments", "Run 'baha version --help' for usage.")
				}
				fmt.Fprintf(out, "baha %s (commit %s, built %s)\n", version, commit, date)
				return nil
			},
		},
	}
	return root
}

func usageError(message, hint string) error {
	return &cli.UsageError{Message: message, Hint: hint}
}

type noArgsHandler func(io.Writer) error

func noArgs(name string, fn noArgsHandler) cli.RunFunc {
	return func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 0 {
			return usageError(name+" does not accept arguments", "Run '"+name+" --help' for usage.")
		}
		return fn(out)
	}
}

type noArgsCtxHandler func(context.Context, io.Writer) error

func noArgsCtx(name string, fn noArgsCtxHandler) cli.RunFunc {
	return func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 0 {
			return usageError(name+" does not accept arguments", "Run '"+name+" --help' for usage.")
		}
		return fn(ctx, out)
	}
}
