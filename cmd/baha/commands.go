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
	for i, child := range appCmd.Children {
		if child.Name == "init" {
			appCmd.Children[i] = appGuidedInitCommand()
			break
		}
	}
	appCmd.Children = append(appCmd.Children,
		appApplyCommand(store),
		appBackupCommand(store),
		appRestoreCommand(store),
		appEnvCommand(store),
		appStatusCommand(store),
		appDoctorCommand(store),
		appDownCommand(store),
		appUpCommand(store),
		appDestroyCommand(store),
		appSecretCommand(store),
		appRuntimeIdentityCommand(store),
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
			Summary: "Start BaseHarbor and, inside an application repository, converge the application",
			Usage:   "baha up [--yes] [--postgres-port PORT] [--openbao-port PORT] [--recovery-file PATH]",
			Long:    "Starts or reuses the local BaseHarbor control plane. When run inside a repository containing baseharbor.yaml, the same command also verifies managed secrets, bootstraps or unseals OpenBao when required, provisions declared backend capabilities, starts the repository workload and verifies readiness. A fresh managed-secret setup requires an operator-selected recovery-file path outside .baseharbor; interactive terminals ask for it, while non-interactive use supplies --recovery-file PATH. Outside an application repository, baha up keeps the control-plane-only behavior.",
			Run:     runtimeUpCommand,
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
		serveCommand(store),
		appCmd,
		openBaoCommand(),
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
