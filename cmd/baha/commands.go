package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

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
			initCmd.Usage = "baha app init [--agents] [--input NAME=VALUE]... [--hostname HOST] [--tls acme|existing|local] [--cert-dir DIR] [--yes] | baha app init [--agents] [NAME] [-e ENV|--environment ENV] [--postgres] [--postgres-instance NAME]... [--redis] [--redis-instance NAME]... [--s3] [--s3-bucket NAME]... [--secrets] [--require-secret NAME]..."
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

	var appPlan *cli.Command
	for _, child := range appCmd.Children {
		if child.Name == "plan" {
			appPlan = child
			break
		}
	}

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
			Usage:   "baha up [-e ENV|--environment ENV] [--yes] [--control-plane-only] [--postgres-port PORT] [--openbao-port PORT] [--recovery-file PATH]",
			Long:    "Starts or reuses the local BaseHarbor control plane. In a detected application project without baseharbor.yaml, interactive use routes into the same guided app-init flow; --yes uses only unambiguous detected values and safe defaults through app init --quick. Once the manifest exists, deployment inputs are resolved from defaults, protected state or explicit automation input and only unresolved required values are requested before apply. A fresh managed-secret setup requires an operator-selected recovery-file path outside .baseharbor; interactive terminals ask for it, while non-interactive use supplies --recovery-file PATH. The repository manifest remains unchanged when -e/--environment selects a deployment context; the override is applied only to resolved runtime state. Directories without application signals keep the control-plane-only behavior. --control-plane-only is an explicit advanced mode for operators and CI that intentionally skips repository application convergence.",
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
			Name:    "plan",
			Summary: "Show the application plan in the current repository",
			Usage:   "baha plan [NAME] [-o json|--output json]",
			Long:    "Repository-aware shorthand for 'baha app plan'. It is read-only and uses the same application planning core.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if appPlan == nil {
					return errors.New("application plan command is unavailable")
				}
				return appPlan.Run(ctx, args, out, errOut)
			},
		},
		{
			Name:    "status",
			Summary: "Show application status in a repository, otherwise control-plane status",
			Usage:   "baha status [-o json|--output json]",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if inApplicationRepository() {
					return appStatusCommandWithTLS(store).Run(ctx, args, out, errOut)
				}
				if len(args) != 0 {
					return usageError("structured application status requires an application repository", "Run inside a repository containing baseharbor.yaml, or use 'baha app status NAME -o json'.")
				}
				return runtimeStatus(ctx, out)
			},
		},
		{
			Name:    "doctor",
			Summary: "Diagnose the current application repository, otherwise the control plane",
			Usage:   "baha doctor [--fix] [-o json|--output json]",
			Long:    "Inside an application repository, runs the same application doctor used by 'baha app doctor'. Outside a repository it keeps the control-plane doctor behavior. Structured output is read-only and cannot be combined with --fix.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				if inApplicationRepository() {
					return appDoctorRepairCommandWithTLS(store).Run(ctx, args, out, errOut)
				}
				if requestsJSONOutput(args) {
					return usageError("structured application doctor requires an application repository", "Run inside a repository containing baseharbor.yaml, or use 'baha app doctor NAME -o json'.")
				}
				return doctorCommand(ctx, args, out, errOut)
			},
		},
		serveCommand(store),
		appCmd,
		connectCommand(),
		disconnectCommand(),
		connectionsCommand(),
		openBaoCommand(),
		updateCommand(),
		{
			Name:    "version",
			Aliases: nil,
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
	root.Children = append(root.Children, tuiCommand(store), completionCommand(root, store), internalCompletionCommand(root, store))
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

func inApplicationRepository() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	_, err = application.FindRepositoryManifest(cwd)
	return err == nil
}
