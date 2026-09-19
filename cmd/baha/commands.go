package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func rootCommand() *cli.Command {
	store := application.DefaultStore()
	appCmd := appCommand(store)
	for i, child := range appCmd.Children {
		switch child.Name {
		case "init":
			initCmd := appInitWithInputResolverCommand(store)
			initCmd.Usage = "baha app init [--input NAME=VALUE]... [--hostname HOST] [--tls acme|existing|local] [--cert-dir DIR] [--yes] | baha app init [NAME] [--environment ENV] [--postgres] [--postgres-instance NAME]... [--redis] [--redis-instance NAME]... [--s3] [--s3-bucket NAME]... [--secrets] [--require-secret NAME]..."
			initCmd.Long += " Without baseharbor.yaml, the existing manifest flags remain available for deterministic repository-contract creation."
			appCmd.Children[i] = initCmd
		case "show":
			appCmd.Children[i] = appShowCommandWithRecoveryMetadata(store)
		}
	}
	appCmd.Children = append(appCmd.Children,
		appInspectCommand(),
		appApplyCommand(store),
		appGuidedBackupCommand(store),
		appGuidedRestoreCommandWithRecoveryMetadata(store),
		appEnvCommand(store),
		appPSQLCommand(store),
		appRedisCommand(store),
		appCredsCommand(store),
		appLogsCommand(store),
		appShellCommand(store),
		appExecCommand(store),
		appUpdateCommand(store),
		appTLSCommand(store),
		appStatusCommandWithTLS(store),
		appDoctorRepairCommandWithTLS(store),
		appDownCommand(store),
		appUpCommand(store),
		appDestroyCommand(store),
		appSecretCommand(store),
		appRuntimeIdentityCommand(store),
	)
	applyRemainingApplicationRuntimeProviderGuards(store, appCmd)

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
			Usage:   "baha up [--yes] [--control-plane-only] [--postgres-port PORT] [--openbao-port PORT] [--recovery-file PATH]",
			Long:    "Starts or reuses the local BaseHarbor control plane. In a detected application project without baseharbor.yaml, interactive use routes into the same guided app-init flow; --yes uses only unambiguous detected values and safe defaults through app init --quick. Once the manifest exists, deployment inputs are resolved from defaults, protected state or explicit automation input and only unresolved required values are requested before apply. A fresh managed-secret setup requires an operator-selected recovery-file path outside .baseharbor; interactive terminals ask for it, while non-interactive use supplies --recovery-file PATH. Directories without application signals keep the control-plane-only behavior. --control-plane-only is an explicit advanced mode for operators and CI that intentionally skips repository application convergence.",
			Run:     runtimeUpCommandWithInputResolver,
		},
		{
			Name:    "down",
			Summary: "Stop the local BaseHarbor control-plane runtime",
			Usage:   "baha down",
			Run:     noArgsCtx("baha down", runtimeDown),
		},
		{
			Name:    "destroy",
			Summary: "Permanently remove the global BaseHarbor control plane and its owned state",
			Usage:   "baha destroy [--yes]",
			Long:    "Shows a destruction plan for the global BaseHarbor Compose project, its owned volumes, runtime state and provider-registry metadata. Refuses to run while application bindings remain. Application-owned repository data and volumes are not removed.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				return runtimeDestroy(ctx, args, out)
			},
		},
		{
			Name:    "status",
			Summary: "Show control-plane container and readiness status",
			Usage:   "baha status",
			Run:     noArgsCtx("baha status", runtimeStatus),
		},
		{
			Name:    "doctor",
			Summary: "Verify prerequisites and safely repair supported runtime findings",
			Usage:   "baha doctor [--fix]",
			Long:    "Classifies failed checks as auto-fixable, fixable with confirmation, requiring developer input, or requiring manual/admin action. --fix only applies safe reversible repairs to existing runtime state and never invents credentials, unseals OpenBao without recovery material, discards data, or silently overwrites application files.",
			Run:     doctorCommand,
		},
		serveCommand(store),
		appCmd,
		openBaoCommand(),
		updateCommand(),
		{
			Name:    "version",
			Aliases: []string{"--version", "-v"},
			Summary: "Print build version",
			Usage:   "baha version",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if len(args) != 0 {
					return usageError("baha version does not accept arguments", "Run '"+"baha version --help' for usage.")
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
